package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

var ErrInvalidJSON = errors.New("invalid JSON")

type Store struct {
	DirectoryMode os.FileMode
	FileMode      os.FileMode
	ScriptMode    os.FileMode
}

func NewStore(directoryMode, fileMode os.FileMode) Store {
	scriptMode := os.FileMode(0o755)
	if directoryMode == 0o700 {
		scriptMode = 0o700
	}
	return Store{DirectoryMode: directoryMode, FileMode: fileMode, ScriptMode: scriptMode}
}

func (s Store) ReadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return nil
}

func (s Store) WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), s.DirectoryMode); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".rotari-tmp-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(s.FileMode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func DefaultMeta() model.Meta {
	return model.Meta{Phase: "collecting", UpdatedAt: ""}
}

func LoadMeta(path string) (model.Meta, error) {
	var meta model.Meta
	store := NewStore(0o700, 0o600)
	if err := store.ReadJSON(path, &meta); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.Meta{Phase: "collecting", UpdatedAt: ""}, nil
		}
		return model.Meta{}, err
	}
	if meta.Phase == "" {
		meta = model.Meta{Phase: "collecting", UpdatedAt: ""}
	}
	return meta, nil
}

func LoadQueue(path string) (model.Queue, error) {
	var queue model.Queue
	store := NewStore(0o700, 0o600)
	if err := store.ReadJSON(path, &queue); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.Queue{}, nil
		}
		return model.Queue{}, err
	}
	return queue, nil
}

func WriteJSON(path string, value any) error {
	return NewStore(0o700, 0o600).WriteJSON(path, value)
}
