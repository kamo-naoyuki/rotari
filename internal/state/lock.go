package state

import (
	"encoding/json"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func LoadLock(path string) (model.LockInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.LockInfo{}, err
	}
	var lock model.LockInfo
	if err := json.Unmarshal(data, &lock); err != nil {
		return model.LockInfo{}, err
	}
	return lock, nil
}
