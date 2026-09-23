package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

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
	return json.Unmarshal(data, value)
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
