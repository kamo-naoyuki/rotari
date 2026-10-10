package runlineage

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestCompareSources(t *testing.T) {
	git := func(commit string, dirty bool) model.SourceRevision {
		return model.SourceRevision{Root: "/repo", VCS: "git", CommitID: commit, Dirty: dirty}
	}
	jj := func(commit string) model.SourceRevision {
		return model.SourceRevision{Root: "/repo", VCS: "jj", CommitID: commit, ChangeID: "kx"}
	}
	runs := func(revisions ...model.SourceRevision) *model.RunSources {
		return &model.RunSources{Sources: revisions}
	}
	for _, test := range []struct {
		name     string
		from, to *model.RunSources
		want     string
	}{
		{name: "same git commit", from: runs(git("a", false)), to: runs(git("a", false)), want: SourceUnchanged},
		{name: "new git commit", from: runs(git("a", false)), to: runs(git("b", true)), want: SourceChanged},
		{name: "same git commit with uncommitted changes", from: runs(git("a", false)), to: runs(git("a", true)), want: SourceUnknown},
		{name: "jj snapshot changed", from: runs(jj("a")), to: runs(jj("b")), want: SourceChanged},
		{name: "jj snapshot unchanged", from: runs(jj("a")), to: runs(jj("a")), want: SourceUnchanged},
		{name: "vcs changed", from: runs(git("a", false)), to: runs(jj("a")), want: SourceUnknown},
		{name: "read failed", from: runs(git("a", false)), to: runs(model.SourceRevision{Root: "/repo", VCS: "git", Error: "git: failed"}), want: SourceUnknown},
		{name: "not recorded in the second run", from: runs(git("a", false)), to: runs(), want: SourceUnknown},
		{name: "second run predates sources", from: runs(git("a", false)), to: nil, want: SourceUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			changes := CompareSources(test.from, test.to)
			if len(changes) != 1 || changes[0].Root != "/repo" || changes[0].Change != test.want {
				t.Fatalf("CompareSources = %+v, want one %q change", changes, test.want)
			}
		})
	}
	if changes := CompareSources(nil, nil); changes != nil {
		t.Fatalf("CompareSources of runs without sources = %+v, want none", changes)
	}
}

// TestCompareCarriesSources checks that a comparison and its run infos carry
// what each run recorded.
func TestCompareCarriesSources(t *testing.T) {
	from := Run{ID: "a", Sources: &model.RunSources{Sources: []model.SourceRevision{{Root: "/repo", VCS: "git", CommitID: "1"}}}}
	to := Run{ID: "b", Sources: &model.RunSources{Sources: []model.SourceRevision{{Root: "/repo", VCS: "git", CommitID: "2"}}}}
	result := Compare(from, to)
	if len(result.Sources) != 1 || result.Sources[0].Change != SourceChanged || len(result.From.Sources) != 1 || result.To.Sources[0].CommitID != "2" {
		t.Fatalf("Compare = %+v", result)
	}
}
