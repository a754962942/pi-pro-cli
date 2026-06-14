package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

type Strategy string

const (
	StrategyDirectReplace Strategy = "direct-replace"
	StrategyWindowsHelper Strategy = "windows-helper"
)

type ApplyRequest struct {
	CurrentExecutablePath string
	ManagedBinaryPath     string
	StagedBinaryPath      string
	HelperPath            string
	ManagedRootPath       string
	ExpectedSHA256        string
	TargetVersion         string
	ParentPID             int
	GOOS                  string
}

var startHelperProcess = func(helperPath string, statePath string) error {
	return exec.Command(helperPath, statePath).Start()
}

func StrategyForOS(goos string) Strategy {
	if goos == "windows" {
		return StrategyWindowsHelper
	}
	return StrategyDirectReplace
}

func Apply(request ApplyRequest) error {
	goos := request.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if err := ValidateManagedExecutable(request.CurrentExecutablePath, request.ManagedBinaryPath); err != nil {
		return err
	}

	switch StrategyForOS(goos) {
	case StrategyWindowsHelper:
		return startWindowsHelper(request)
	default:
		return replaceDirect(request.StagedBinaryPath, request.ManagedBinaryPath)
	}
}

func ValidateManagedExecutable(currentPath string, managedPath string) error {
	current, err := normalizedPath(currentPath)
	if err != nil {
		return err
	}
	managed, err := normalizedPath(managedPath)
	if err != nil {
		return err
	}
	if current == managed {
		return nil
	}

	return apperror.AppError{
		Code:    errdefs.CodeUpdateUnsupportedInstallLocation,
		Message: "CLI binary is not running from the managed install location.",
		Kind:    apperror.KindLifecycle,
		Details: map[string]any{
			"currentExecutable": current,
			"managedBinary":     managed,
		},
	}
}

func replaceDirect(stagedPath string, managedPath string) error {
	if err := os.Rename(stagedPath, managedPath); err != nil {
		return apperror.AppError{
			Code:    errdefs.CodeUpdateReplaceFailed,
			Message: "failed to replace CLI binary",
			Kind:    apperror.KindLifecycle,
			Details: map[string]any{
				"error": err.Error(),
			},
		}
	}
	return nil
}

func startWindowsHelper(request ApplyRequest) error {
	if request.HelperPath == "" {
		return helperMissing()
	}
	if _, err := os.Stat(request.HelperPath); err != nil {
		return helperMissing()
	}

	statePath := filepath.Join(request.ManagedRootPath, "updates", "update-state.json")
	if err := WriteState(statePath, UpdateState{
		ManagedRootPath:   request.ManagedRootPath,
		ManagedBinaryPath: request.ManagedBinaryPath,
		StagedBinaryPath:  request.StagedBinaryPath,
		ExpectedSHA256:    request.ExpectedSHA256,
		TargetVersion:     request.TargetVersion,
		ParentPID:         request.ParentPID,
		Status:            StateStatusPending,
	}); err != nil {
		return err
	}

	if err := startHelperProcess(request.HelperPath, statePath); err != nil {
		return apperror.AppError{
			Code:    errdefs.CodeUpdateHelperFailed,
			Message: "failed to start Windows helper updater",
			Kind:    apperror.KindLifecycle,
			Details: map[string]any{
				"error": err.Error(),
			},
		}
	}
	return nil
}

func helperMissing() error {
	return apperror.AppError{
		Code:    errdefs.CodeUpdateHelperMissing,
		Message: "Windows helper updater is missing.",
		Kind:    apperror.KindLifecycle,
	}
}

func normalizedPath(path string) (string, error) {
	if path == "" {
		return "", apperror.AppError{
			Code:    errdefs.CodeUpdateUnsupportedInstallLocation,
			Message: "CLI executable path is empty.",
			Kind:    apperror.KindLifecycle,
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned), nil
	}
	return cleaned, nil
}

func RunHelper(statePath string) error {
	state, err := ReadState(statePath)
	if err != nil {
		return err
	}
	if err := validateStatePaths(statePath, state); err != nil {
		return err
	}
	if err := waitForParentExit(state.ParentPID); err != nil {
		_ = markStateFailed(statePath, state, err)
		return err
	}
	if err := replaceWithBackup(state.ManagedBinaryPath, state.StagedBinaryPath); err != nil {
		_ = markStateFailed(statePath, state, err)
		return err
	}
	if !fileMatchesSHA256(state.ManagedBinaryPath, state.ExpectedSHA256) {
		err := apperror.AppError{
			Code:    errdefs.CodeUpdateReplaceFailed,
			Message: "installed update binary checksum did not match update state",
			Kind:    apperror.KindLifecycle,
		}
		_ = markStateFailed(statePath, state, err)
		return err
	}
	_ = os.Remove(state.StagedBinaryPath)
	state.Status = StateStatusCompleted
	state.Error = ""
	return WriteState(statePath, state)
}

func validateStatePaths(statePath string, state UpdateState) error {
	if state.ManagedRootPath == "" || state.ManagedBinaryPath == "" || state.StagedBinaryPath == "" || state.ExpectedSHA256 == "" {
		return invalidState("update state is missing required fields")
	}
	for _, path := range []string{statePath, state.ManagedBinaryPath, state.StagedBinaryPath} {
		ok, err := isWithinRoot(path, state.ManagedRootPath)
		if err != nil {
			return err
		}
		if !ok {
			return invalidState("update state path escapes managed root")
		}
	}
	return nil
}

func invalidState(message string) error {
	return apperror.AppError{
		Code:    errdefs.CodeUpdateStateInvalid,
		Message: message,
		Kind:    apperror.KindLifecycle,
	}
}

func isWithinRoot(path string, root string) (bool, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

func waitForParentExit(pid int) error {
	if pid <= 0 || runtime.GOOS != "windows" {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return apperror.AppError{
			Code:    errdefs.CodeUpdateHelperFailed,
			Message: "failed to find parent process",
			Kind:    apperror.KindLifecycle,
			Details: map[string]any{
				"error": err.Error(),
			},
		}
	}
	_, err = process.Wait()
	if err != nil {
		return apperror.AppError{
			Code:    errdefs.CodeUpdateHelperFailed,
			Message: "failed while waiting for parent process to exit",
			Kind:    apperror.KindLifecycle,
			Details: map[string]any{
				"error": err.Error(),
			},
		}
	}
	return nil
}

func replaceWithBackup(managedPath string, stagedPath string) error {
	backupPath := fmt.Sprintf("%s.bak-%d", managedPath, os.Getpid())
	backupCreated := false
	if _, err := os.Stat(managedPath); err == nil {
		if err := os.Rename(managedPath, backupPath); err != nil {
			return replaceFailed(err)
		}
		backupCreated = true
	}
	if err := os.Rename(stagedPath, managedPath); err != nil {
		if backupCreated {
			_ = os.Rename(backupPath, managedPath)
		}
		return replaceFailed(err)
	}
	if backupCreated {
		_ = os.Remove(backupPath)
	}
	return nil
}

func replaceFailed(err error) error {
	return apperror.AppError{
		Code:    errdefs.CodeUpdateReplaceFailed,
		Message: "failed to replace CLI binary",
		Kind:    apperror.KindLifecycle,
		Details: map[string]any{
			"error": err.Error(),
		},
	}
}

func fileMatchesSHA256(path string, expected string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == expected
}

func markStateFailed(statePath string, state UpdateState, failure error) error {
	state.Status = StateStatusFailed
	state.Error = failure.Error()
	return WriteState(statePath, state)
}
