package state

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	// codeql[go/path-injection]: callers provide paths under the validated state root.
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
	// codeql[go/path-injection]: callers provide paths under the validated state root.
	if err := os.MkdirAll(filepath.Dir(path), s.DirectoryMode); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".rotari-tmp-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	// codeql[go/path-injection]: temporaryName is created by os.CreateTemp in the target directory.
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
	// codeql[go/path-injection]: path is supplied by callers after state-path validation.
	return os.Rename(temporaryName, path)
}

func DefaultMeta() model.Meta {
	return model.Meta{Phase: "collecting", UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
}

func LoadMeta(path string) (model.Meta, error) {
	var meta model.Meta
	store := NewStore(DirectoryMode(), FileMode())
	if err := store.ReadJSON(path, &meta); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultMeta(), nil
		}
		return model.Meta{}, err
	}
	if meta.Phase == "" {
		meta.Phase = "collecting"
	}
	if meta.UpdatedAt == "" {
		meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return meta, nil
}

func LoadQueue(path string) (model.Queue, error) {
	var queue model.Queue
	store := NewStore(DirectoryMode(), FileMode())
	if err := store.ReadJSON(path, &queue); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.Queue{}, nil
		}
		return model.Queue{}, err
	}
	return queue, nil
}

func WriteJSON(path string, value any) error {
	return NewStore(DirectoryMode(), FileMode()).WriteJSON(path, value)
}

func AppendLoadSample(path string, sample model.LoadSample) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(sample)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}

func ReadLoadSamples(path string) []model.LoadSample {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var samples []model.LoadSample
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var sample model.LoadSample
		if json.Unmarshal(line, &sample) == nil {
			samples = append(samples, sample)
		}
	}
	return samples
}
