package main

import (
	"bytes"
	"os"
	"testing"
)

func TestTextTableAlignsColoredAndRightAlignedCells(t *testing.T) {
	oldCheck := terminalCheck
	terminalCheck = func(*os.File) bool { return false }
	defer func() { terminalCheck = oldCheck }()

	table := textTable{
		columns: []tableColumn{{header: "STATE"}, {header: "N", alignRight: true}, {header: "NAME"}},
		rows:    [][]string{{"success", "7", "a"}, {"failed", "12", "long-name"}},
	}
	var output bytes.Buffer
	table.write(&output)
	want := "STATE     N  NAME\nsuccess   7  a\nfailed   12  long-name\n"
	if output.String() != want {
		t.Fatalf("table =\n%s\nwant\n%s", output.String(), want)
	}

	// Escape codes do not count toward a column's width.
	terminalCheck = func(*os.File) bool { return true }
	table.columns[0].color = func(_ int, value string) string { return colorJobsState(value) }
	output.Reset()
	table.write(&output)
	want = ansiCyan + "STATE     N  NAME" + ansiReset + "\n" +
		ansiGreen + "success" + ansiReset + "   7  a\n" +
		ansiRed + "failed" + ansiReset + "   12  long-name\n"
	if output.String() != want {
		t.Fatalf("colored table = %q, want %q", output.String(), want)
	}
}
