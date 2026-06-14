package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

func TestValidateManagedExecutableAcceptsManagedPath(t *testing.T) {
	managed := filepath.Join(t.TempDir(), "config", "bin", "pi-pro")

	if err := ValidateManagedExecutable(managed, managed); err != nil {
		t.Fatalf("expected managed executable to be accepted, got %v", err)
	}
}

func TestValidateManagedExecutableRejectsDifferentPath(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	managed := filepath.Join(configDir, "bin", "pi-pro")
	current := filepath.Join(t.TempDir(), "other", "pi-pro")

	err := ValidateManagedExecutable(current, managed)

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeUpdateUnsupportedInstallLocation {
		t.Fatalf("expected %s, got %q", errdefs.CodeUpdateUnsupportedInstallLocation, appErr.Code)
	}
}

func TestStrategyForOSBranchesWindows(t *testing.T) {
	if StrategyForOS("windows") != StrategyWindowsHelper {
		t.Fatalf("expected windows helper strategy")
	}
	if StrategyForOS("darwin") != StrategyDirectReplace {
		t.Fatalf("expected direct replace strategy")
	}
	if StrategyForOS("linux") != StrategyDirectReplace {
		t.Fatalf("expected direct replace strategy")
	}
}

func TestApplyWindowsWritesStateAndStartsHelper(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	managed := filepath.Join(configDir, "bin", "pi-pro.exe")
	staged := filepath.Join(configDir, "updates", "pi-pro-0.1.1.exe")
	helper := filepath.Join(configDir, "bin", "pi-pro-updater.exe")
	if err := os.MkdirAll(filepath.Dir(helper), 0o700); err != nil {
		t.Fatalf("expected helper dir, got %v", err)
	}
	if err := os.WriteFile(helper, []byte("helper"), 0o700); err != nil {
		t.Fatalf("expected helper file, got %v", err)
	}

	var startedHelper string
	var startedState string
	originalStarter := startHelperProcess
	startHelperProcess = func(helperPath string, statePath string) error {
		startedHelper = helperPath
		startedState = statePath
		return nil
	}
	t.Cleanup(func() {
		startHelperProcess = originalStarter
	})

	err := Apply(ApplyRequest{
		CurrentExecutablePath: managed,
		ManagedBinaryPath:     managed,
		StagedBinaryPath:      staged,
		HelperPath:            helper,
		ManagedRootPath:       configDir,
		ExpectedSHA256:        "abc123",
		TargetVersion:         "0.1.1",
		ParentPID:             12345,
		GOOS:                  "windows",
	})

	if err != nil {
		t.Fatalf("expected Windows apply to start helper, got %v", err)
	}
	if startedHelper != helper {
		t.Fatalf("expected helper %q, got %q", helper, startedHelper)
	}
	if startedState == "" {
		t.Fatalf("expected helper state path")
	}
	var state UpdateState
	data, err := os.ReadFile(startedState)
	if err != nil {
		t.Fatalf("expected update-state.json, got %v", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("expected valid update state, got %v", err)
	}
	if state.ManagedBinaryPath != managed || state.StagedBinaryPath != staged || state.ExpectedSHA256 != "abc123" || state.TargetVersion != "0.1.1" || state.ParentPID != 12345 {
		t.Fatalf("unexpected update state %#v", state)
	}
}

func TestRunHelperReplacesManagedBinaryAndCleansStagedFile(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	managed := filepath.Join(configDir, "bin", "pi-pro.exe")
	staged := filepath.Join(configDir, "updates", "pi-pro-0.1.1.exe")
	newBinary := []byte("new binary")
	if err := os.MkdirAll(filepath.Dir(managed), 0o700); err != nil {
		t.Fatalf("expected managed dir, got %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(staged), 0o700); err != nil {
		t.Fatalf("expected staged dir, got %v", err)
	}
	if err := os.WriteFile(managed, []byte("old binary"), 0o700); err != nil {
		t.Fatalf("expected managed binary, got %v", err)
	}
	if err := os.WriteFile(staged, newBinary, 0o700); err != nil {
		t.Fatalf("expected staged binary, got %v", err)
	}
	statePath := filepath.Join(configDir, "updates", "update-state.json")
	if err := WriteState(statePath, UpdateState{
		ManagedRootPath:   configDir,
		ManagedBinaryPath: managed,
		StagedBinaryPath:  staged,
		ExpectedSHA256:    testSHA256Hex(newBinary),
		TargetVersion:     "0.1.1",
		Status:            StateStatusPending,
	}); err != nil {
		t.Fatalf("expected state write, got %v", err)
	}

	if err := RunHelper(statePath); err != nil {
		t.Fatalf("expected helper to succeed, got %v", err)
	}
	got, err := os.ReadFile(managed)
	if err != nil {
		t.Fatalf("expected managed binary, got %v", err)
	}
	if string(got) != string(newBinary) {
		t.Fatalf("expected managed binary %q, got %q", newBinary, got)
	}
	if _, err := os.Stat(staged); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected staged binary cleanup, got %v", err)
	}

	state, err := ReadState(statePath)
	if err != nil {
		t.Fatalf("expected state read, got %v", err)
	}
	if state.Status != StateStatusCompleted {
		t.Fatalf("expected completed state, got %#v", state)
	}
}

func TestRunHelperRejectsPathsOutsideManagedRoot(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	outside := filepath.Join(t.TempDir(), "outside", "pi-pro.exe")
	staged := filepath.Join(configDir, "updates", "pi-pro-0.1.1.exe")
	statePath := filepath.Join(configDir, "updates", "update-state.json")
	if err := WriteState(statePath, UpdateState{
		ManagedRootPath:   configDir,
		ManagedBinaryPath: outside,
		StagedBinaryPath:  staged,
		ExpectedSHA256:    "abc123",
		TargetVersion:     "0.1.1",
		Status:            StateStatusPending,
	}); err != nil {
		t.Fatalf("expected state write, got %v", err)
	}

	err := RunHelper(statePath)

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeUpdateStateInvalid {
		t.Fatalf("expected %s, got %q", errdefs.CodeUpdateStateInvalid, appErr.Code)
	}
}

func testSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
