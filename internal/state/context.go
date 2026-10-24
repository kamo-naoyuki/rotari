package state

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/rotari-dev/rotari/internal/model"
)

func ContextPath(runDir string) string {
	return filepath.Join(runDir, "context.json")
}

func LoadContext(store Store, runDir string) (model.RunContext, error) {
	var context model.RunContext
	if err := store.ReadJSON(ContextPath(runDir), &context); err != nil {
		return model.RunContext{}, err
	}
	return context, nil
}

func SaveContext(store Store, runDir string, context model.RunContext) error {
	return store.WriteJSON(ContextPath(runDir), context)
}

// LoadRunStartedBy reads how the run in runDir was started, nil when its
// context is missing or predates the record.
func LoadRunStartedBy(store Store, runDir string) (*model.StartedBy, error) {
	context, err := LoadContext(store, runDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return context.StartedBy, nil
}
