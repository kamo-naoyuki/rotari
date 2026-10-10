package model

import "testing"

func TestSourceForTakesTheInnermostRepository(t *testing.T) {
	sources := RunSources{Sources: []SourceRevision{{Root: "/work"}, {Root: "/work/vendor/lib"}}}
	for dir, want := range map[string]string{
		"/work":                "/work",
		"/work/src":            "/work",
		"/work/vendor/lib/a":   "/work/vendor/lib",
		"/work-other":          "",
		"/workspace/unrelated": "",
		"/":                    "",
	} {
		source, ok := sources.SourceFor(dir)
		if (want == "") == ok || source.Root != want {
			t.Errorf("SourceFor(%q) = %+v, %v; want %q", dir, source, ok, want)
		}
	}
}

func TestFormatSourceRevision(t *testing.T) {
	for _, test := range []struct {
		revision SourceRevision
		want     string
	}{
		{SourceRevision{VCS: "git", CommitID: "89281f8c3a1b2c3d4e5f"}, "git 89281f8c3a1b (clean)"},
		{SourceRevision{VCS: "git", CommitID: "89281f8c3a1b2c3d4e5f", Dirty: true}, "git 89281f8c3a1b (uncommitted changes)"},
		{SourceRevision{VCS: "jj", CommitID: "1a2b3c4d5e6f7a8b", ChangeID: "kxqzmwuotplsvyrn"}, "jj 1a2b3c4d5e6f (change kxqzmwuotpls)"},
		{SourceRevision{VCS: "git", Error: "git: fatal"}, "git unknown (git: fatal)"},
	} {
		if got := FormatSourceRevision(test.revision); got != test.want {
			t.Errorf("FormatSourceRevision(%+v) = %q, want %q", test.revision, got, test.want)
		}
	}
	if got := SourceLabel(SourceRevision{Root: "/work", VCS: "git", CommitID: "abc"}); got != "git abc (clean) in /work" {
		t.Errorf("SourceLabel = %q", got)
	}
}
