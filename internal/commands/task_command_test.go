package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestTaskStatusCommandOutputsJSON(t *testing.T) {
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != serverapi.TaskStatus("job_123") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"running"}`), nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"task", "status", "job_123"}, &stdout, &stderr, Options{ServerURL: "https://api.example.test", HTTPClient: httpClient})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	var body struct {
		OK     bool   `json:"ok"`
		JobID  string `json:"jobId"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || body.JobID != "job_123" || body.Status != "running" {
		t.Fatalf("unexpected status output %#v", body)
	}
	if stderr.String() != "" {
		t.Fatalf("expected no stderr for one-shot status, got %q", stderr.String())
	}
}

func TestTaskWaitCommandWritesProgressOnlyToStderr(t *testing.T) {
	calls := 0
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"running"}`), nil
		}
		return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"succeeded"}`), nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"task", "wait", "job_123", "--timeout", "30", "--poll-interval", "1", "--poll-max", "1", "--poll-backoff", "1", "--no-jitter"}, &stdout, &stderr, Options{
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
		TaskSleep:  func(delay time.Duration) {},
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s stderr %s", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "task job_123 running") {
		t.Fatalf("expected progress on stderr, got %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "polling again") {
		t.Fatalf("expected stdout to contain JSON only, got %q", stdout.String())
	}
	var body struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON stdout, got %v", err)
	}
	if !body.OK || body.Status != "succeeded" {
		t.Fatalf("unexpected wait output %#v", body)
	}
}

func TestTaskCancelCommandCallsServer(t *testing.T) {
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != serverapi.TaskCancel("job_123") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"cancelled"}`), nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"task", "cancel", "job_123"}, &stdout, &stderr, Options{ServerURL: "https://api.example.test", HTTPClient: httpClient})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	var body struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || body.Status != "cancelled" {
		t.Fatalf("unexpected cancel output %#v", body)
	}
}

func TestTaskCommandRequiresJobID(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Execute([]string{"task", "status"}, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit")
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if body.Error.Code != errdefs.CodeUsage {
		t.Fatalf("expected %s, got %q", errdefs.CodeUsage, body.Error.Code)
	}
}

func TestTaskWaitCommandTimeoutReturnsJSONError(t *testing.T) {
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"running"}`), nil
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := ExecuteWithOptions([]string{"task", "wait", "job_123", "--timeout", "0", "--poll-interval", "1", "--poll-max", "1", "--poll-backoff", "1", "--no-jitter"}, &stdout, &stderr, Options{
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
		TaskSleep:  func(delay time.Duration) {},
	})

	if exitCode == 0 {
		t.Fatalf("expected timeout exit")
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON error, got %v", err)
	}
	if body.Error.Code != errdefs.CodeTaskTimeout {
		t.Fatalf("expected %s, got %q", errdefs.CodeTaskTimeout, body.Error.Code)
	}
}
