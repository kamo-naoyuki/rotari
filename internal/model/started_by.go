package model

import (
	"fmt"
	"strings"
	"unicode"
)

// StartedBy is how a run was started: the interface its request came
// through, which is always recorded, and an actor label the starter gave
// with ROTARI_ACTOR or --actor, such as an agent's name. rotari never
// guesses an actor; a run without one is unlabelled, not a person's.
type StartedBy struct {
	Interface string `json:"interface"`
	Actor     string `json:"actor,omitempty"`
}

// The interfaces a run can be started through.
const (
	StartedByCLI = "cli"
	StartedByMCP = "mcp"
)

// MaxActorLength is the most characters an actor label may have.
const MaxActorLength = 64

// NormalizeActor trims an actor label and rejects one that is too long or
// holds a control character.
func NormalizeActor(label string) (string, error) {
	label = strings.TrimSpace(label)
	if len([]rune(label)) > MaxActorLength {
		return "", fmt.Errorf("actor label is longer than %d characters", MaxActorLength)
	}
	if strings.IndexFunc(label, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("actor label %q contains a control character", label)
	}
	return label, nil
}

// Label is how a run's start is shown: "cli", or "cli · claude-code" with
// an actor.
func (startedBy StartedBy) Label() string {
	if startedBy.Actor == "" {
		return startedBy.Interface
	}
	return startedBy.Interface + " · " + startedBy.Actor
}
