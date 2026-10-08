package state

import (
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

const RunClientStatusFileName = "client_status.json"

// LoadRunClientStatus reads the initiating client's persisted status for a run.
func LoadRunClientStatus(store Store, runDir string) (model.RunClientStatus, error) {
	path, err := ValidatedStateFile(runDir, RunClientStatusFileName)
	if err != nil {
		return model.RunClientStatus{}, err
	}
	var status model.RunClientStatus
	if err := store.ReadJSON(path, &status); err != nil {
		return model.RunClientStatus{}, err
	}
	return status, nil
}

// WriteRunClientStatus persists the initiating client's last known status.
func WriteRunClientStatus(store Store, runDir string, status model.RunClientStatus) error {
	// Job IDs remain arbitrary valid path elements. Optional metadata must
	// never occupy the path of a job or change whether that job can execute.
	commands, err := LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return err
	}
	for _, job := range model.QueueToJobs(commands.Commands) {
		if job.ID == RunClientStatusFileName {
			return nil
		}
	}
	path, err := ValidatedStateFile(runDir, RunClientStatusFileName)
	if err != nil {
		return err
	}
	return store.WriteJSON(filepath.Clean(path), status)
}
