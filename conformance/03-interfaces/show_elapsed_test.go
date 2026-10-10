package interfaces

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// TestShowReportsElapsedAndQuietTime runs one job that writes output and then
// sleeps, and one that never writes output. While they run, the run table,
// show -j, and jobs report how long each has run and how long ago it last
// wrote output, or that it wrote none.
func TestShowReportsElapsedAndQuietTime(t *testing.T) {
	covers(t, "CLI-23")
	e := support.NewEnv(t)
	project := "elapsed"
	quiet := support.AddedJobID(t, e.MustRotari("add", "-p", project, "--job-name", "quiet", "--", "sh", "-c", "echo started; sleep 300"))
	silent := support.AddedJobID(t, e.MustRotari("add", "-p", project, "--job-name", "silent", "--", "sleep", "300"))
	e.MustRotari("run", "-p", project, "--async", "--quiet")
	t.Cleanup(func() { e.Rotari("cancel", "-p", project, "--wait") })

	quietRow := regexp.MustCompile(`(?m)^` + quiet + `\s.*\s\d+s, quiet \d+s\s`)
	silentRow := regexp.MustCompile(`(?m)^` + silent + `\s.*\s\d+s, no output\s`)
	deadline := time.Now().Add(15 * time.Second)
	var table string
	for {
		table = e.MustRotari("show", "-p", project, "--no-pager").Stdout
		if quietRow.MatchString(table) && silentRow.MatchString(table) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("show does not report elapsed and quiet time of the running jobs:\n%s", table)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !strings.Contains(table, "ELAPSED") {
		t.Fatalf("show table has no ELAPSED column:\n%s", table)
	}
	job := e.MustRotari("show", "-p", project, "-j", quiet, "--no-pager").Stdout
	if !regexp.MustCompile(`(?m)^Elapsed: \d+s, quiet \d+s$`).MatchString(job) {
		t.Fatalf("show -j does not report elapsed and quiet time:\n%s", job)
	}

	jobs := e.MustRotari("jobs", project).Stdout
	for _, row := range []*regexp.Regexp{
		regexp.MustCompile(`(?m)\squiet\s.*\s\d+s, quiet \d+s$`),
		regexp.MustCompile(`(?m)\ssilent\s.*\s\d+s, no output$`),
	} {
		if !row.MatchString(jobs) {
			t.Fatalf("jobs does not report elapsed and quiet time of the running jobs (want %s):\n%s", row, jobs)
		}
	}
	var listed []struct {
		JobName      string   `json:"job_name"`
		QuietSeconds *float64 `json:"quiet_seconds"`
	}
	output := e.MustRotari("jobs", project, "--json").Stdout
	if err := json.Unmarshal([]byte(output), &listed); err != nil {
		t.Fatalf("jobs --json: %v\n%s", err, output)
	}
	quietSeconds := map[string]*float64{}
	for _, row := range listed {
		quietSeconds[row.JobName] = row.QuietSeconds
	}
	if seconds, ok := quietSeconds["quiet"]; !ok || seconds == nil || *seconds < 0 {
		t.Fatalf("jobs --json gives the quiet job no quiet_seconds:\n%s", output)
	}
	if seconds, ok := quietSeconds["silent"]; !ok || seconds != nil {
		t.Fatalf("jobs --json gives the silent job quiet_seconds:\n%s", output)
	}
}
