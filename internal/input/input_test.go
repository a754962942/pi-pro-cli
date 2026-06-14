package input

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

func TestLoadMergesInputJSONOverCLIValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, []byte(`{"prompt":"from json","duration":5}`), 0o600); err != nil {
		t.Fatalf("expected input file write, got %v", err)
	}

	got, err := Load(Options{
		InputPath: path,
		CLIValues: map[string]any{
			"prompt":     "from cli",
			"resolution": "720p",
		},
	})

	if err != nil {
		t.Fatalf("expected input load, got %v", err)
	}
	if got["prompt"] != "from json" {
		t.Fatalf("expected input JSON to win, got %#v", got)
	}
	if got["resolution"] != "720p" || got["duration"] != float64(5) {
		t.Fatalf("expected merged values, got %#v", got)
	}
}

func TestLoadReadsInputFromStdin(t *testing.T) {
	got, err := Load(Options{
		InputPath: "-",
		Stdin:     strings.NewReader(`{"prompt":"from stdin"}`),
	})

	if err != nil {
		t.Fatalf("expected stdin input, got %v", err)
	}
	if got["prompt"] != "from stdin" {
		t.Fatalf("expected stdin prompt, got %#v", got)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	_, err := Load(Options{
		InputPath: "-",
		Stdin:     strings.NewReader(`{`),
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeInvalidInputJSON {
		t.Fatalf("expected %s, got %q", errdefs.CodeInvalidInputJSON, appErr.Code)
	}
}

func TestLoadRejectsNonObjectJSON(t *testing.T) {
	_, err := Load(Options{
		InputPath: "-",
		Stdin:     strings.NewReader(`["not-object"]`),
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeInvalidInputJSON {
		t.Fatalf("expected %s, got %q", errdefs.CodeInvalidInputJSON, appErr.Code)
	}
}

func TestLoadWithoutInputReturnsCLIValues(t *testing.T) {
	got, err := Load(Options{
		CLIValues: map[string]any{"cameraFixed": false, "seed": 0},
	})

	if err != nil {
		t.Fatalf("expected cli input, got %v", err)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("expected marshal, got %v", err)
	}
	if !strings.Contains(string(data), `"cameraFixed":false`) || !strings.Contains(string(data), `"seed":0`) {
		t.Fatalf("expected false and 0 to be preserved, got %s", data)
	}
}
