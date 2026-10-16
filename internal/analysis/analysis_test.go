package analysis

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestPipeline(t *testing.T) {
	c := fixture(t, "c.csv", "Neighborhood,CrimeDateTime,CCNumber,Description\nDowntown,1/1/2025 1:00:00 PM,A,ROBBERY\n DOWNTOWN ,2/1/2025 1:00:00 PM,A,ASSAULT\nDowntown,1/1/2026 1:00:00 PM,B,ROBBERY\n,1/1/2025 1:00:00 PM,C,ROBBERY\n")
	v := fixture(t, "v.csv", "Neighborhood,DateNotice,BLOCKLOT\ndowntown,2026/01/01 00:00:00+00,1\n")
	for _, workers := range []int{1, 4} {
		r, err := Load(c, v, 2025, workers)
		if err != nil {
			t.Fatal(err)
		}
		n := r.Neighborhoods["DOWNTOWN"]
		if r.ReadCrime != 4 || r.IncludedCrime != 3 || r.MissingNeighborhood != 1 || r.LaterNotices != 1 || n.Records != 2 || len(n.Cases) != 1 || n.Notices != 1 || n.Months[0] != 1 || n.Months[1] != 1 {
			t.Fatalf("unexpected result: %+v %+v", r, n)
		}
		var b bytes.Buffer
		Report(&b, r, 2025, 10)
		if !strings.Contains(b.String(), "5. Which areas") {
			t.Fatal("missing report")
		}
	}
}
func TestRejectBadInputs(t *testing.T) {
	v := fixture(t, "v.csv", "Neighborhood,DateNotice,BLOCKLOT\n")
	for _, body := range []string{"Neighborhood\nX\n", "Neighborhood,CrimeDateTime,CCNumber,Description\nX,bad,A,ROBBERY\n"} {
		c := fixture(t, "c.csv", body)
		if _, err := Load(c, v, 2025, 2); err == nil {
			t.Fatal("expected input error")
		}
	}
	if _, err := Load("missing", "missing", 2025, 0); err == nil {
		t.Fatal("expected worker error")
	}
}

func TestConcentration(t *testing.T) {
	r := &Result{Neighborhoods: map[string]*Neighborhood{
		"A": {Name: "A", Notices: 3, Cases: map[string]bool{"shared": true}},
		"B": {Name: "B", Notices: 1, Cases: map[string]bool{"shared": true, "second": true}},
		"C": {Name: "C", Cases: map[string]bool{"third": true}},
	}}
	var b bytes.Buffer
	reportConcentration(&b, r)
	for _, want := range []string{"Top  10% (  1 of   2)", "75.0%", "33.3%", "66.7%", "100.0%"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %q in %s", want, b.String())
		}
	}
	r.Neighborhoods["A"].Cases = nil
	r.Neighborhoods["B"].Cases = nil
	r.Neighborhoods["C"].Cases = nil
	b.Reset()
	reportConcentration(&b, r)
	if !strings.Contains(b.String(), "N/A") {
		t.Fatal("zero denominator must be N/A")
	}
	b.Reset()
	reportConcentration(&b, &Result{Neighborhoods: map[string]*Neighborhood{}})
	if !strings.Contains(b.String(), "unavailable") {
		t.Fatal("empty notices must be reported")
	}
}

func TestPartitionedJoinMatchesSerial(t *testing.T) {
	var crime, vacant strings.Builder
	crime.WriteString("Neighborhood,CrimeDateTime,CCNumber,Description\n")
	vacant.WriteString("Neighborhood,DateNotice,BLOCKLOT\n")
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&crime, "Area %d,3/1/2025 1:00:00 PM,case%d,ROBBERY\n", i%31, i%100)
		if i%2 == 0 {
			fmt.Fprintf(&vacant, " area %d ,2024/01/01 00:00:00+00,parcel%d\n", i%31, i)
		}
	}
	c := fixture(t, "crime.csv", crime.String())
	v := fixture(t, "vacant.csv", vacant.String())
	serial, err := Load(c, v, 2025, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, workers := range []int{2, 4, 8} {
		parallel, err := Load(c, v, 2025, workers)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(serial, parallel) {
			t.Fatalf("results differ with %d workers", workers)
		}
	}
	if len(serial.Neighborhoods) != 31 || serial.Notices != 500 || serial.IncludedCrime != 1000 {
		t.Fatalf("unexpected totals: %+v", serial)
	}
}
