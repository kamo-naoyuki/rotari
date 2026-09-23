package state

import (
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
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
