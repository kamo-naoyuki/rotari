package runlineage

import "github.com/kamo-naoyuki/rotari/internal/model"

// Source comparison outcomes.
const (
	SourceUnchanged = "unchanged"
	SourceChanged   = "changed"
	// SourceUnknown means the record cannot tell: a side did not record the
	// repository, a read failed, or a git working tree had uncommitted
	// changes, so the same commit may hold different code.
	SourceUnknown = "unknown"
)

// SourceChange compares one repository's revision in two runs.
type SourceChange struct {
	Root   string                `json:"root"`
	From   *model.SourceRevision `json:"from,omitempty"`
	To     *model.SourceRevision `json:"to,omitempty"`
	Change string                `json:"change"`
}

// CompareSources compares the repositories that either run recorded, in the
// order the runs list them. A run that recorded no sources at all gives no
// comparison, as it predates source recording or ran outside any repository.
func CompareSources(from, to *model.RunSources) []SourceChange {
	if from == nil && to == nil {
		return nil
	}
	changes := []SourceChange{}
	index := make(map[string]int)
	add := func(revision model.SourceRevision, isFrom bool) {
		position, ok := index[revision.Root]
		if !ok {
			position = len(changes)
			index[revision.Root] = position
			changes = append(changes, SourceChange{Root: revision.Root})
		}
		copied := revision
		if isFrom {
			changes[position].From = &copied
		} else {
			changes[position].To = &copied
		}
	}
	if from != nil {
		for _, revision := range from.Sources {
			add(revision, true)
		}
	}
	if to != nil {
		for _, revision := range to.Sources {
			add(revision, false)
		}
	}
	for position := range changes {
		changes[position].Change = sourceChange(changes[position].From, changes[position].To)
	}
	return changes
}

func sourceChange(from, to *model.SourceRevision) string {
	if from == nil || to == nil || from.Error != "" || to.Error != "" || from.VCS != to.VCS {
		return SourceUnknown
	}
	if from.CommitID != to.CommitID {
		return SourceChanged
	}
	if from.Dirty || to.Dirty {
		return SourceUnknown
	}
	return SourceUnchanged
}

// CombineSourceChanges sums up a comparison over repositories: changed when
// any repository's code changed, otherwise unknown when any cannot be told,
// otherwise unchanged; empty when there is nothing to compare.
func CombineSourceChanges(changes []SourceChange) string {
	if len(changes) == 0 {
		return ""
	}
	combined := SourceUnchanged
	for _, change := range changes {
		switch change.Change {
		case SourceChanged:
			return SourceChanged
		case SourceUnknown:
			combined = SourceUnknown
		}
	}
	return combined
}
