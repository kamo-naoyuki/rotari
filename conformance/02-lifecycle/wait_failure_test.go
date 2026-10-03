package lifecycle

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestWaitUntilFailureReturnsAtAFinalFailure(t *testing.T) {
	covers(t, "RUN-6")
	e := support.NewEnv(t)
	failing := support.AddedJobID(t, e.MustRotari("add", "-p", "early", "--", "sh", "-c", "exit 3"))
	e.MustRotari("add", "-p", "early", "--", "sleep", "20")
	e.MustRotari("run", "-p", "early", "--async", "--quiet")
	t.Cleanup(func() { e.Rotari("cancel", "-p", "early", "--wait") })

	started := time.Now()
	result := e.Rotari("wait", "-p", "early", "--until-failure", "--timeout", "15s")
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("wait --until-failure took %s, want it to return before the sleeping job ends", elapsed)
	}
	if result.Code != 1 || !strings.Contains(result.Stdout, "is still running") || !strings.Contains(result.Stdout, "Failures by cause:") {
		t.Fatalf("wait --until-failure = %s, want exit 1 with the failure groups of the running run", result)
	}

	var early struct {
		Status   string `json:"status"`
		Failures []struct {
			Jobs []struct {
				ID string `json:"id"`
			} `json:"jobs"`
		} `json:"failures"`
	}
	result = e.Rotari("wait", "-p", "early", "--until-failure", "--json", "--timeout", "15s")
	if err := json.Unmarshal([]byte(result.Stdout), &early); err != nil || result.Code != 1 {
		t.Fatalf("wait --until-failure --json = %s: %v", result, err)
	}
	if early.Status != "running" || len(early.Failures) != 1 || len(early.Failures[0].Jobs) != 1 || early.Failures[0].Jobs[0].ID != failing {
		t.Fatalf("wait --until-failure --json = %+v, want the running run with job %s failed", early, failing)
	}
}

func TestWaitUntilFailureIgnoresAFailureTheRunRetries(t *testing.T) {
	covers(t, "RUN-6")
	e := support.NewEnv(t)
	marker := filepath.Join(e.Root, "attempts")
	// The first attempt fails and the retry, three seconds later, succeeds,
	// so wait sees the failed attempt for several polls before the retry.
	e.MustRotari("add", "-p", "retried", "--retry", "1", "--retry-delay", "3s", "--", "sh", "-c",
		fmt.Sprintf("printf x >> %q; test $(wc -c < %q) -ge 2", marker, marker))
	e.MustRotari("run", "-p", "retried", "--async", "--quiet")

	result := e.Rotari("wait", "-p", "retried", "--until-failure", "--timeout", "30s")
	if result.Code != 0 || strings.Contains(result.Stdout, "is still running") {
		t.Fatalf("wait --until-failure = %s, want the run's successful completion", result)
	}
}
