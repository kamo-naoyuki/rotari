package resolution

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestDisplayTimesFollowTZ(t *testing.T) {
	covers(t, "RES-11")
	e := support.NewEnv(t)
	runID, _ := e.FinishedJobRun("p")
	var shown struct {
		Summary struct {
			StartedAt string `json:"started_at"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "p", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	started, err := time.Parse(time.RFC3339, shown.Summary.StartedAt)
	if err != nil || !strings.HasSuffix(shown.Summary.StartedAt, "Z") {
		t.Fatalf("show --json started_at %q is not UTC RFC3339: %v", shown.Summary.StartedAt, err)
	}
	for _, zone := range []string{"UTC", "Asia/Tokyo", "America/New_York"} {
		t.Run(zone, func(t *testing.T) {
			location, err := time.LoadLocation(zone)
			if err != nil {
				t.Skipf("time zone data unavailable: %v", err)
			}
			want := started.In(location).Format("2006-01-02 15:04:05 MST")
			zoned := e.WithVar("TZ", zone)
			if output := zoned.MustRotari("show", "-p", "p").Stdout; !strings.Contains(output, want) {
				t.Errorf("show does not print the start time as %q:\n%s", want, output)
			}
			got := zoned.HTTPGet(zoned.StartWeb() + "/api/state")
			var state struct {
				Projects []struct {
					Runs []struct {
						RunID     string `json:"run_id"`
						StartedAt string `json:"started_at"`
					} `json:"runs"`
				} `json:"projects"`
			}
			if err := json.Unmarshal([]byte(got.Body), &state); err != nil {
				t.Fatalf("GET /api/state: status %d: %v", got.Status, err)
			}
			found := false
			for _, project := range state.Projects {
				for _, webRun := range project.Runs {
					if webRun.RunID == runID {
						found = true
						if webRun.StartedAt != want {
							t.Errorf("Web API started_at = %q, want %q", webRun.StartedAt, want)
						}
					}
				}
			}
			if !found {
				t.Errorf("Web API state has no run %s", runID)
			}
		})
	}
}
