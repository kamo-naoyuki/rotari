package state

import (
	"encoding/json"
	"os"
	"syscall"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func ProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

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
