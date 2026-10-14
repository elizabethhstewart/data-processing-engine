// Package analysis implements a bounded, concurrent CSV analysis pipeline.
package analysis

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type Neighborhood struct {
	Name    string
	Notices int
	Records int
	Cases   map[string]bool
	Types   map[string]int
	Months  [12]int
}
type Result struct {
	Neighborhoods                                                        map[string]*Neighborhood
	ReadCrime, IncludedCrime, MissingNeighborhood, Notices, LaterNotices int
}
type event struct {
	name, caseID, kind string
	month              int
	notice             bool
}

func Normalize(s string) string { return strings.Join(strings.Fields(strings.ToUpper(s)), " ") }
func (r *Result) get(name string) *Neighborhood {
	n := r.Neighborhoods[name]
	if n == nil {
		n = &Neighborhood{Name: name, Cases: map[string]bool{}, Types: map[string]int{}}
		r.Neighborhoods[name] = n
	}
	return n
}

// Load uses two readers and bounded worker pools. Only the collector owns maps,
// avoiding locks and concurrent map writes. Raw rows are not retained.
func Load(crime, vacant string, year, workers int) (*Result, error) {
	if workers < 1 || workers > 64 {
		return nil, fmt.Errorf("workers must be between 1 and 64")
	}
	if year < 2000 || year > 2100 {
		return nil, fmt.Errorf("year must be between 2000 and 2100")
	}
	out := make(chan event, 256)
	errs := make(chan error, 2)
	var readers sync.WaitGroup
	for _, spec := range []struct {
		path   string
		notice bool
	}{{crime, false}, {vacant, true}} {
		readers.Add(1)
		go func(path string, notice bool) { defer readers.Done(); errs <- read(path, notice, year, workers, out) }(spec.path, spec.notice)
	}
	go func() { readers.Wait(); close(out); close(errs) }()
	r := &Result{Neighborhoods: map[string]*Neighborhood{}}
	for e := range out {
		if e.notice {
			r.Notices++
			if e.month == 1 {
				r.LaterNotices++
			}
			if e.name != "" {
				r.get(e.name).Notices++
			}
			continue
		}
		r.ReadCrime++
		if e.month < 0 {
			continue
		}
		r.IncludedCrime++
		if e.name == "" {
			r.MissingNeighborhood++
			continue
		}
		n := r.get(e.name)
		n.Records++
		n.Cases[e.caseID] = true
		n.Types[e.kind]++
		n.Months[e.month]++
	}
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}
func read(path string, notice bool, year, workers int, out chan<- event) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cr := csv.NewReader(f)
	header, err := cr.Read()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	columns := map[string]int{}
	for i, h := range header {
		columns[strings.TrimPrefix(h, "\ufeff")] = i
	}
	required := []string{"Neighborhood", "CrimeDateTime", "CCNumber", "Description"}
	if notice {
		required = []string{"Neighborhood", "DateNotice", "BLOCKLOT"}
	}
	for _, k := range required {
		if _, ok := columns[k]; !ok {
			return fmt.Errorf("%s: missing column %s", path, k)
		}
	}
	jobs := make(chan []string, 256)
	var wg sync.WaitGroup
	var first error
	var mu sync.Mutex
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for row := range jobs {
				e := event{name: Normalize(row[columns["Neighborhood"]]), notice: notice}
				key, layout := "CrimeDateTime", "1/2/2006 3:04:05 PM"
				if notice {
					key, layout = "DateNotice", "2006/01/02 15:04:05-07"
				}
				d, err := time.Parse(layout, row[columns[key]])
				if err != nil {
					mu.Lock()
					if first == nil {
						first = fmt.Errorf("%s: invalid %s: %w", path, key, err)
					}
					mu.Unlock()
					continue
				}
				if notice {
					if d.Year() > year {
						e.month = 1
					}
				} else {
					e.month = -1
					if d.Year() == year {
						e.month = int(d.Month()) - 1
					}
					e.caseID = row[columns["CCNumber"]]
					e.kind = Normalize(row[columns["Description"]])
				}
				out <- e
			}
		}()
	}
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			mu.Lock()
			if first == nil {
				first = fmt.Errorf("%s: %w", path, err)
			}
			mu.Unlock()
			break
		}
		jobs <- row
	}
	close(jobs)
	wg.Wait()
	return first
}
func ranked(r *Result, less func(a, b *Neighborhood) bool) []*Neighborhood {
	var ns []*Neighborhood
	for _, n := range r.Neighborhoods {
		if n.Notices > 0 {
			ns = append(ns, n)
		}
	}
	sort.Slice(ns, func(i, j int) bool {
		if less(ns[i], ns[j]) {
			return true
		}
		if less(ns[j], ns[i]) {
			return false
		}
		return ns[i].Name < ns[j].Name
	})
	return ns
}
func Report(w io.Writer, r *Result, year, top int) {
	fmt.Fprintf(w, "BALTIMORE CRIME AND VACANCY REPORT — %d\nCrime rows read: %d | in year: %d | missing neighborhood: %d\nVacant notice rows: %d | dated after analysis year: %d\n", year, r.ReadCrime, r.IncludedCrime, r.MissingNeighborhood, r.Notices, r.LaterNotices)
	fmt.Fprintln(w, "Crime records can share case numbers. Cases are distinct within each neighborhood.\nVacancies are a supplied snapshot, not a reconstructed historical inventory.\nCounts are not population-adjusted rates; associations do not establish causation.")
	printTop := func(title string, ns []*Neighborhood, value func(*Neighborhood) string) {
		fmt.Fprintln(w, "\n"+title)
		for i, n := range ns {
			if i >= top {
				break
			}
			fmt.Fprintf(w, "%-30s %s\n", n.Name, value(n))
		}
	}
	printTop("1. Where are the most notices, and how many crime records occur there?", ranked(r, func(a, b *Neighborhood) bool { return a.Notices > b.Notices }), func(n *Neighborhood) string {
		return fmt.Sprintf("%5d notices | %5d records | %5d cases", n.Notices, n.Records, len(n.Cases))
	})
	printTop("2. Which neighborhoods with notices have the most distinct crime cases?", ranked(r, func(a, b *Neighborhood) bool { return len(a.Cases) > len(b.Cases) }), func(n *Neighborhood) string { return fmt.Sprintf("%5d cases | %5d notices", len(n.Cases), n.Notices) })
	fmt.Fprintln(w, "\n3. What crime types are reported in neighborhoods with versus without notices?\nTYPE                         WITH NOTICES   WITHOUT NOTICES")
	yes, no := map[string]int{}, map[string]int{}
	for _, n := range r.Neighborhoods {
		dst := no
		if n.Notices > 0 {
			dst = yes
		}
		for k, v := range n.Types {
			dst[k] += v
		}
	}
	keys := map[string]bool{}
	for k := range yes {
		keys[k] = true
	}
	for k := range no {
		keys[k] = true
	}
	var types []string
	for k := range keys {
		types = append(types, k)
	}
	sort.Strings(types)
	for _, k := range types {
		fmt.Fprintf(w, "%-28s %12d %17d\n", k, yes[k], no[k])
	}
	fmt.Fprintln(w, "\n4. How do monthly crime records vary in neighborhoods with notices?")
	var months [12]int
	for _, n := range r.Neighborhoods {
		if n.Notices > 0 {
			for i, v := range n.Months {
				months[i] += v
			}
		}
	}
	max := 1
	for _, v := range months {
		if v > max {
			max = v
		}
	}
	for i, v := range months {
		fmt.Fprintf(w, "%s %6d %s\n", time.Month(i+1), v, strings.Repeat("#", v*40/max))
	}
	maxNot, maxCases := 1, 1
	for _, n := range r.Neighborhoods {
		if n.Notices > maxNot {
			maxNot = n.Notices
		}
		if len(n.Cases) > maxCases {
			maxCases = len(n.Cases)
		}
	}
	score := func(n *Neighborhood) float64 {
		return 50*float64(n.Notices)/float64(maxNot) + 50*float64(len(n.Cases))/float64(maxCases)
	}
	printTop("5. Which areas rank highest on a combined planning score?", ranked(r, func(a, b *Neighborhood) bool { return score(a) > score(b) }), func(n *Neighborhood) string {
		return fmt.Sprintf("%5.1f / 100 | %d notices | %d cases", score(n), n.Notices, len(n.Cases))
	})
	fmt.Fprintln(w, "Score: equal weights for notice count and case count, each divided by its citywide maximum.\nThis exploratory score is not a prediction or an allocation recommendation.")
}
