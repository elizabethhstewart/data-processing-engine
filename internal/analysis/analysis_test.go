package analysis

import (
	"bytes"
	"os"
	"path/filepath"
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
