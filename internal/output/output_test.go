package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
)

func TestWriteJSONWritesSuccessEnvelope(t *testing.T) {
	var stdout strings.Builder

	exitCode := WriteJSON(&stdout, map[string]any{
		"ok":     true,
		"jobId":  "job_123",
		"status": "submitted",
	})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", body["ok"])
	}
	if body["jobId"] != "job_123" {
		t.Fatalf("expected jobId to be preserved, got %#v", body["jobId"])
	}
}

func TestWriteErrorWritesStableEnvelopeAndExitCode(t *testing.T) {
	var stdout strings.Builder

	exitCode := WriteError(&stdout, apperror.AppError{
		Code:    "VALIDATION_ERROR",
		Message: "Input validation failed.",
		Kind:    apperror.KindValidation,
		Details: []map[string]string{
			{"field": "prompt", "message": "prompt is required"},
		},
	})

	if exitCode != 2 {
		t.Fatalf("expected validation exit code 2, got %d", exitCode)
	}

	var body struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Field string `json:"field"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if body.OK {
		t.Fatalf("expected ok=false")
	}
	if body.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected validation code, got %q", body.Error.Code)
	}
	if body.Error.Message != "Input validation failed." {
		t.Fatalf("expected validation message, got %q", body.Error.Message)
	}
	if len(body.Error.Details) != 1 || body.Error.Details[0].Field != "prompt" {
		t.Fatalf("expected structured details, got %#v", body.Error.Details)
	}
}

func TestWriteDiagnosticWritesOnlyToStderr(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	WriteDiagnostic(&stderr, "polling task job_123")

	if stdout.String() != "" {
		t.Fatalf("expected stdout to stay empty, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "polling task job_123") {
		t.Fatalf("expected stderr diagnostic, got %q", stderr.String())
	}
}

func TestSecretsAreRedactedFromErrorOutput(t *testing.T) {
	var stdout strings.Builder

	WriteError(&stdout, apperror.AppError{
		Code:    "AUTH_ERROR",
		Message: "password=super-secret authToken=pi_token_123 Authorization: Bearer abc123",
		Kind:    apperror.KindAuth,
		Details: map[string]any{
			"password":      "super-secret",
			"authToken":     "pi_token_123",
			"Authorization": "Bearer abc123",
			"nested": map[string]any{
				"requestBody": "password=super-secret",
			},
		},
	})

	got := stdout.String()
	for _, secret := range []string{"super-secret", "pi_token_123", "Bearer abc123"} {
		if strings.Contains(got, secret) {
			t.Fatalf("expected secret %q to be redacted from %s", secret, got)
		}
	}
	if !strings.Contains(got, "redacted") {
		t.Fatalf("expected redacted marker in %s", got)
	}
}

func TestStringMapDetailsAreRedacted(t *testing.T) {
	var stdout strings.Builder

	WriteError(&stdout, apperror.AppError{
		Code:    "AUTH_ERROR",
		Message: "auth failed",
		Kind:    apperror.KindAuth,
		Details: map[string]string{
			"password": "super-secret",
			"field":    "safe",
		},
	})

	got := stdout.String()
	if strings.Contains(got, "super-secret") {
		t.Fatalf("expected password to be redacted from %s", got)
	}
	if !strings.Contains(got, "safe") {
		t.Fatalf("expected non-secret value to be preserved in %s", got)
	}
}

func TestExitCodeMapping(t *testing.T) {
	tests := []struct {
		name string
		kind apperror.Kind
		want int
	}{
		{name: "usage", kind: apperror.KindUsage, want: 2},
		{name: "validation", kind: apperror.KindValidation, want: 2},
		{name: "auth", kind: apperror.KindAuth, want: 3},
		{name: "network", kind: apperror.KindNetwork, want: 4},
		{name: "task", kind: apperror.KindTask, want: 5},
		{name: "io", kind: apperror.KindIO, want: 6},
		{name: "lifecycle", kind: apperror.KindLifecycle, want: 7},
		{name: "unknown", kind: apperror.Kind("unknown"), want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout strings.Builder

			got := WriteError(&stdout, apperror.AppError{
				Code:    "ERROR",
				Message: "error",
				Kind:    tt.kind,
			})

			if got != tt.want {
				t.Fatalf("expected exit code %d, got %d", tt.want, got)
			}
		})
	}
}
