package commands

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestHelpListsPlannedMVPCommands(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Execute([]string{"--help"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	for _, want := range []string{
		"pi-pro init",
		"pi-pro update",
		"pi-pro auth login",
		"pi-pro auth logout",
		"pi-pro auth status",
		"pi-pro types list",
		"pi-pro types inspect --provider <provider> --model <model> --type <type>",
		"pi-pro task status <jobId>",
		"pi-pro task wait <jobId>",
		"pi-pro task cancel <jobId>",
		"pi-pro generateImage --provider <provider> --model <model> --type <type>",
		"pi-pro generateVoice --provider <provider> --model <model> --type <type>",
		"pi-pro generateVideo --provider <provider> --model <model> --type <type>",
		"pi-pro --version",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected help output to contain %q, got:\n%s", want, stdout.String())
		}
	}
}

func TestVersionReturnsStableJSON(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Execute([]string{"--version"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected valid JSON, got error %v and output %q", err, stdout.String())
	}
	if body["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", body["ok"])
	}
	if body["localVersion"] == "" {
		t.Fatalf("expected non-empty localVersion, got %#v", body["localVersion"])
	}
	if _, ok := body["commit"]; ok {
		t.Fatalf("did not expect commit in version output: %#v", body)
	}
}

func TestInitCommandRunsLifecycle(t *testing.T) {
	schema := []byte(`{"ok":true}`)
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return commandJSONResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.0"}`), nil
		case serverapi.CLIInitManifest:
			return commandJSONResponse(http.StatusOK, `{"version":"2026-06-14","files":[{"path":"schemas/file.json","url":"`+serverapi.CLIFilesPrefix+`file.json","sha256":"`+commandSHA256Hex(schema)+`","required":true}]}`), nil
		case serverapi.CLIFilesPrefix + "file.json":
			return commandBytesResponse(http.StatusOK, schema), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"init"}, &stdout, &stderr, Options{
		ConfigDir:    filepath.Join(t.TempDir(), "config"),
		ServerURL:    "https://api.example.test",
		LocalVersion: "0.1.0",
		HTTPClient:   httpClient,
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	var body struct {
		OK          bool `json:"ok"`
		Initialized bool `json:"initialized"`
		Files       struct {
			Downloaded int `json:"downloaded"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || !body.Initialized || body.Files.Downloaded != 1 {
		t.Fatalf("unexpected init output %#v", body)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestUpdateCommandRunsLifecycle(t *testing.T) {
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != serverapi.CLIVersion {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return commandJSONResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.0"}`), nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"update"}, &stdout, &stderr, Options{
		ConfigDir:    filepath.Join(t.TempDir(), "config"),
		ServerURL:    "https://api.example.test",
		LocalVersion: "0.1.0",
		HTTPClient:   httpClient,
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	var body struct {
		OK      bool `json:"ok"`
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || body.Changed {
		t.Fatalf("unexpected update output %#v", body)
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestUnknownCommandFailsWithUsageError(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Execute([]string{"unknown"}, &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	var body struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected valid JSON, got error %v and output %q", err, stdout.String())
	}
	if body.OK {
		t.Fatalf("expected ok=false")
	}
	if body.Error.Code != errdefs.CodeUnknownCommand {
		t.Fatalf("expected %s, got %q", errdefs.CodeUnknownCommand, body.Error.Code)
	}
}

type commandHTTPClient func(req *http.Request) (*http.Response, error)

func (f commandHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func commandJSONResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func commandBytesResponse(statusCode int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func commandSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
