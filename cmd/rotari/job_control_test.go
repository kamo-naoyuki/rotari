package main

import (
	"bytes"
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/resolve"
)

func parseJobControlOptions(t *testing.T, states []string, args ...string) *jobControlOptions {
	t.Helper()
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	options := cliJobControlOptions(fs, states)
	if err := cliParse(fs, args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return options
}

func TestJobControlOptionsSelectByNameOrFilter(t *testing.T) {
	selection, filtered, err := parseJobControlOptions(t, cancelStates, "--job-name", "train").selection()
	if err != nil || filtered || len(selection.Names) != 1 {
		t.Fatalf("--job-name selection = %+v, %t, %v", selection, filtered, err)
	}
	selection, filtered, err = parseJobControlOptions(t, cancelStates, "--stage", "fit", "--filter-host", "gpu-*", "--filter-state", "pending").selection()
	if err != nil || !filtered || selection.Scope.Stage != "fit" || len(selection.Filter.Hosts) != 1 || selection.States[0] != "pending" {
		t.Fatalf("filtered selection = %+v, %t, %v", selection, filtered, err)
	}
	if _, _, err := parseJobControlOptions(t, cancelStates, "--job-name", "train", "--stage", "fit").selection(); err == nil {
		t.Fatal("--job-name was combined with --stage")
	}
}

func TestJobControlOptionsRejectOtherFilters(t *testing.T) {
	for _, args := range [][]string{
		{"--filter-state", "pending"},
		{"--filter-exit-code", "1"},
		{"--filter-finished-after", "1d"},
		{"--failed"},
	} {
		fs := flag.NewFlagSet("suspend", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		cliJobControlOptions(fs, signalStates)
		if err := cliParse(fs, args); err == nil {
			t.Errorf("suspend accepted %v", args)
		}
	}
}

func TestResolveJobControlRejectsJobIDsWithFilters(t *testing.T) {
	options := parseJobControlOptions(t, cancelStates, "--filter-command", "train")
	_, err := resolveJobControl(resolve.JobControl{JobIDs: []string{"job-1"}}, options, cancelStates, "cancel")
	if err == nil || !strings.Contains(err.Error(), "job IDs cannot be combined") {
		t.Fatalf("resolveJobControl error = %v", err)
	}
	target := resolve.JobControl{JobIDs: []string{"job-1"}}
	got, err := resolveJobControl(target, parseJobControlOptions(t, cancelStates), cancelStates, "cancel")
	if err != nil || len(got.JobIDs) != 1 || got.JobIDs[0] != "job-1" {
		t.Fatalf("unfiltered target = %+v, %v", got, err)
	}
}

func TestConfirmJobControl(t *testing.T) {
	jobs := []string{"job-a", "job-b"}
	if err := confirmJobControl(strings.NewReader(""), io.Discard, false, true, "cancel", "run-1", jobs); err != nil {
		t.Fatalf("--yes error = %v", err)
	}
	if err := confirmJobControl(strings.NewReader(""), io.Discard, false, false, "cancel", "run-1", jobs); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("non-terminal error = %v", err)
	}
	var output bytes.Buffer
	if err := confirmJobControl(strings.NewReader("y\n"), &output, true, false, "suspend", "run-1", jobs); err != nil {
		t.Fatalf("confirmed error = %v", err)
	}
	if !strings.Contains(output.String(), "suspend 2 job(s) of run run-1") || !strings.Contains(output.String(), "  job-b\n") {
		t.Fatalf("prompt = %q", output.String())
	}
	if err := confirmJobControl(strings.NewReader("n\n"), io.Discard, true, false, "cancel", "run-1", jobs); err == nil || err.Error() != "cancel cancelled" {
		t.Fatalf("declined error = %v", err)
	}
}

// TestCancelWaitWithSelectionSaysHowToFollowTheRun checks that cancel
// rejects --wait with a job selection before resolving anything, and that
// the error says what to run instead.
func TestCancelWaitWithSelectionSaysHowToFollowTheRun(t *testing.T) {
	for _, args := range [][]string{
		{"--wait", "job-1"},
		{"--wait", "--job-name", "train"},
		{"--wait", "--filter-state", "running", "--yes"},
	} {
		code, stderr := captureStderr(t, func() int { return cmdCancel(args) })
		if code != 1 || !strings.Contains(stderr, "--wait may not be used with a job selection") || !strings.Contains(stderr, "rotari wait") {
			t.Errorf("cmdCancel(%q) = %d, stderr %q; want a rejection that points to rotari wait", args, code, stderr)
		}
	}
}
