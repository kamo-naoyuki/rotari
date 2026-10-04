package main

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
)

func TestWriteArtifactListing(t *testing.T) {
	entries := func(count int) []jobstatus.ArtifactEntry {
		var list []jobstatus.ArtifactEntry
		for index := range count {
			path := "/work/out/" + strconv.Itoa(index) + ".csv"
			list = append(list, jobstatus.ArtifactEntry{Path: path, DisplayPath: path, Type: jobstatus.ArtifactMissing, Origin: "argument"})
		}
		return list
	}
	tests := []struct {
		name    string
		listing jobstatus.ArtifactListing
		limit   int
		want    string
	}{
		{name: "not recorded", listing: jobstatus.ArtifactListing{}, limit: shownArtifacts,
			want: "Artifacts: (not recorded)\n"},
		{name: "none found", listing: jobstatus.ArtifactListing{Recorded: true}, limit: shownArtifacts,
			want: "Artifacts: none found\n"},
		{name: "relative to the working directory", listing: jobstatus.ArtifactListing{Recorded: true, WorkingDirectory: "/work", Entries: []jobstatus.ArtifactEntry{
			{Path: "/work/results", DisplayPath: "results", Type: jobstatus.ArtifactDirectory, Origin: "a.yaml: out_dir"},
			{Path: "/etc/hostname", DisplayPath: "/etc/hostname", Type: jobstatus.ArtifactFile, Origin: "argument"},
			{Path: "/workspace/x.csv", DisplayPath: "/workspace/x.csv", Type: jobstatus.ArtifactMissing, Origin: "--in"},
			{Path: "rel.csv", DisplayPath: "rel.csv", Type: jobstatus.ArtifactUnknown, Origin: "argument"},
		}}, limit: shownArtifacts,
			want: "Artifacts: relative to /work\n" +
				"  directory  results  (a.yaml: out_dir)\n" +
				"  file       /etc/hostname  (argument)\n" +
				"  missing    /workspace/x.csv  (--in)\n" +
				"  unknown    rel.csv  (argument)\n"},
		{name: "limited", listing: jobstatus.ArtifactListing{Recorded: true, Entries: entries(3),
			Diagnostics: []artifact.Diagnostic{{Source: "/x.yaml", Message: "not inspected: no such file"}}}, limit: 2,
			want: "Artifacts:\n" +
				"  missing    /work/out/0.csv  (argument)\n" +
				"  missing    /work/out/1.csv  (argument)\n" +
				"  ... and 1 more: rotari show -j att_x --artifacts\n"},
		{name: "full with diagnostics", listing: jobstatus.ArtifactListing{Recorded: true, Entries: entries(1),
			Diagnostics: []artifact.Diagnostic{{Source: "/x.yaml", Message: "not inspected: no such file"}}}, limit: 0,
			want: "Artifacts:\n" +
				"  missing    /work/out/0.csv  (argument)\n" +
				"Discovery notes:\n" +
				"  /x.yaml: not inspected: no such file\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			writeArtifactListing(&output, test.listing, test.limit, "rotari show -j att_x --artifacts")
			if output.String() != test.want {
				t.Fatalf("output:\n%s\nwant:\n%s", output.String(), test.want)
			}
		})
	}
}
