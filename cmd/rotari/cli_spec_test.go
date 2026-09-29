package main

import "testing"

func TestCliSimilarCommand(t *testing.T) {
	if got := cliSimilarCommand("lieneage"); got != "lineage" {
		t.Fatalf("similar command = %q, want lineage", got)
	}
	if got := cliSimilarCommand("shwo"); got != "show" {
		t.Fatalf("similar command = %q, want show", got)
	}
	if got := cliSimilarCommand("completely-unrelated"); got != "" {
		t.Fatalf("unrelated command suggestion = %q, want none", got)
	}
}
