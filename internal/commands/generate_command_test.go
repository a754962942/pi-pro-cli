package commands

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/a754962942/pi-pro-cli/internal/config"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestGenerateVideoCommandSubmitsNoWait(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-test", Username: "user@example.com"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	writeGenerateCommandSchema(t, config.PathsFor(configDir).SchemasDir, "mock-provider", "mock-model", "text-to-video", "video")
	var received map[string]any
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != serverapi.Generations {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-pipro-test" {
			t.Fatalf("expected bearer token, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("expected generation request body, got %v", err)
		}
		return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateVideo", "--provider", "mock-provider", "--model", "mock-model", "--type", "text-to-video", "--input", "-", "--no-wait"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		CommandStdin: strings.NewReader(`{"prompt":"lake"}`),
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	var body struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
		JobID  string `json:"jobId"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON stdout, got %v", err)
	}
	if !body.OK || body.Status != "submitted" || body.JobID != "job_123" {
		t.Fatalf("unexpected generate output %#v", body)
	}
	if received["artifactKind"] != "video" {
		t.Fatalf("expected video artifact kind, got %#v", received)
	}
}

func TestGenerateCommandRequiresProviderModelType(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Execute([]string{"generateVideo", "--provider", "mock-provider"}, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit")
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON error, got %v", err)
	}
	if body.Error.Code != errdefs.CodeUsage {
		t.Fatalf("expected %s, got %q", errdefs.CodeUsage, body.Error.Code)
	}
}

func TestGenerateCommandWaitsByDefault(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-test"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	writeGenerateCommandSchema(t, config.PathsFor(configDir).SchemasDir, "mock-provider", "mock-model", "text-to-image", "image")
	calls := 0
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == http.MethodPost && r.URL.Path == serverapi.Generations {
			return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.TaskStatus("job_123") {
			return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"succeeded","artifacts":[{"url":"https://server.example/image.png","kind":"image"}]}`), nil
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		return nil, nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateImage", "--provider", "mock-provider", "--model", "mock-model", "--type", "text-to-image", "--input", "-", "--timeout", "30", "--poll-interval", "1", "--poll-max", "1", "--poll-backoff", "1", "--no-jitter"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		CommandStdin: strings.NewReader(`{"prompt":"lake"}`),
		TaskSleep:    func(delay time.Duration) {},
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if calls != 2 {
		t.Fatalf("expected submit and poll, got %d calls", calls)
	}
	var body struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON stdout, got %v", err)
	}
	if !body.OK || body.Status != "succeeded" {
		t.Fatalf("unexpected waited output %#v", body)
	}
}

func writeGenerateCommandSchema(t *testing.T, root string, provider string, model string, schemaType string, artifactKind string) {
	t.Helper()
	path := filepath.Join(root, provider, model, schemaType+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("expected schema dir, got %v", err)
	}
	body := `{
  "provider": "` + provider + `",
  "model": "` + model + `",
  "type": "` + schemaType + `",
  "schemaVersion": "1.0",
  "artifactKind": "` + artifactKind + `",
  "input": {
    "required": ["prompt"],
    "properties": {
      "prompt": {"type": "string", "minLength": 1}
    }
  }
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("expected schema write, got %v", err)
	}
}
