package updater

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	StateStatusPending   = "pending"
	StateStatusCompleted = "completed"
	StateStatusFailed    = "failed"
)

type UpdateState struct {
	ManagedRootPath   string `json:"managedRootPath"`
	ManagedBinaryPath string `json:"managedBinaryPath"`
	StagedBinaryPath  string `json:"stagedBinaryPath"`
	ExpectedSHA256    string `json:"expectedSha256"`
	TargetVersion     string `json:"targetVersion"`
	ParentPID         int    `json:"parentPid"`
	Status            string `json:"status"`
	Error             string `json:"error,omitempty"`
}

func ReadState(path string) (UpdateState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return UpdateState{}, err
	}
	var state UpdateState
	if err := json.Unmarshal(data, &state); err != nil {
		return UpdateState{}, err
	}
	return state, nil
}

func WriteState(path string, state UpdateState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
