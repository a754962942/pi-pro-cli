package commands

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/config"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestTypesListCommandReturnsLocalSchemas(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	writeCommandSchema(t, config.PathsFor(configDir).SchemasDir, "seeddance", "v1", "image-to-video")
	writeCommandSchema(t, config.PathsFor(configDir).SchemasDir, "seeddance", "v1", "two-image-to-video")
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"types", "list"}, &stdout, &stderr, Options{ConfigDir: configDir})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	var body struct {
		OK    bool `json:"ok"`
		Types []struct {
			Provider     string `json:"provider"`
			Model        string `json:"model"`
			Type         string `json:"type"`
			ArtifactKind string `json:"artifactKind"`
			DisplayName  string `json:"displayName"`
		} `json:"types"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || len(body.Types) != 2 {
		t.Fatalf("unexpected list output %#v", body)
	}
	if body.Types[0].Provider != "seeddance" || body.Types[0].Model != "v1" || body.Types[0].Type != "image-to-video" || body.Types[0].ArtifactKind != "video" {
		t.Fatalf("unexpected type summary %#v", body.Types[0])
	}
	if body.Types[1].Provider != "seeddance" || body.Types[1].Model != "v1" || body.Types[1].Type != "two-image-to-video" || body.Types[1].ArtifactKind != "video" {
		t.Fatalf("unexpected type summary %#v", body.Types[1])
	}
}

func TestTypesListCommandPrefersCapabilityAPIWhenServerConfigured(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != serverapi.CapabilityTypes {
			t.Fatalf("expected capability types request, got %s", r.URL.Path)
		}
		return commandJSONResponse(http.StatusOK, `{"types":[{"code":"image-to-video","name":"首帧生视频","artifactKind":"video"},{"code":"two-image-to-video","name":"首尾帧生视频","artifactKind":"video"}]}`), nil
	})

	exitCode := ExecuteWithOptions([]string{"types", "list"}, &stdout, &stderr, Options{
		ConfigDir:  filepath.Join(t.TempDir(), "config"),
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	var body struct {
		OK    bool `json:"ok"`
		Types []struct {
			Type         string `json:"type"`
			DisplayName  string `json:"displayName"`
			ArtifactKind string `json:"artifactKind"`
		} `json:"types"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || len(body.Types) != 2 ||
		body.Types[0].Type != "image-to-video" ||
		body.Types[0].DisplayName != "首帧生视频" ||
		body.Types[1].Type != "two-image-to-video" {
		t.Fatalf("unexpected capability list output %#v", body)
	}
}

func TestTypesListCommandFallsBackToLocalSchemasWhenCapabilityAPIUnavailable(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	writeCommandSchema(t, config.PathsFor(configDir).SchemasDir, "seeddance", "v1", "image-to-video")
	var stdout strings.Builder
	var stderr strings.Builder
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})

	exitCode := ExecuteWithOptions([]string{"types", "list"}, &stdout, &stderr, Options{
		ConfigDir:  configDir,
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	var body struct {
		OK    bool `json:"ok"`
		Types []struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Type     string `json:"type"`
		} `json:"types"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || len(body.Types) != 1 || body.Types[0].Provider != "seeddance" || body.Types[0].Type != "image-to-video" {
		t.Fatalf("unexpected fallback list output %#v", body)
	}
}

func TestTypesInspectCommandReturnsSelectedSchema(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	writeCommandSchema(t, config.PathsFor(configDir).SchemasDir, "seeddance", "v1", "image-to-video")
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"types", "inspect", "--provider", "seeddance", "--model", "v1", "--type", "image-to-video"}, &stdout, &stderr, Options{ConfigDir: configDir})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	var body struct {
		OK     bool `json:"ok"`
		Schema struct {
			Provider      string         `json:"provider"`
			Model         string         `json:"model"`
			Type          string         `json:"type"`
			SchemaVersion string         `json:"schemaVersion"`
			Input         map[string]any `json:"input"`
		} `json:"schema"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || body.Schema.Provider != "seeddance" || body.Schema.Type != "image-to-video" || body.Schema.SchemaVersion != "1.0" {
		t.Fatalf("unexpected inspect output %#v", body)
	}
	if body.Schema.Input == nil {
		t.Fatalf("expected full schema input in inspect output")
	}
}

func TestTypesInspectCommandReturnsCapabilityModelsForType(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != serverapi.CapabilityModels("image-to-video") {
			t.Fatalf("expected capability models request, got %s", r.URL.Path)
		}
		return commandJSONResponse(http.StatusOK, `{"models":[{"code":"grok-video-1.0","name":"Grok Video","modality":"video","supportedEventTypes":["image-to-video"],"providers":[{"providerCode":"grok","providerModelId":"grok-imagine-video-1.5-preview","healthStatus":"healthy"}]}]}`), nil
	})

	exitCode := ExecuteWithOptions([]string{"types", "inspect", "--type", "image-to-video"}, &stdout, &stderr, Options{
		ConfigDir:  filepath.Join(t.TempDir(), "config"),
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	if strings.Contains(stdout.String(), "apiKey") || strings.Contains(stdout.String(), "baseUrl") {
		t.Fatalf("expected public capability output without secrets, got %s", stdout.String())
	}
	var body struct {
		OK     bool   `json:"ok"`
		Type   string `json:"type"`
		Models []struct {
			Code      string `json:"code"`
			Providers []struct {
				ProviderCode string `json:"providerCode"`
			} `json:"providers"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK ||
		body.Type != "image-to-video" ||
		len(body.Models) != 1 ||
		body.Models[0].Code != "grok-video-1.0" ||
		len(body.Models[0].Providers) != 1 ||
		body.Models[0].Providers[0].ProviderCode != "grok" {
		t.Fatalf("unexpected capability inspect output %#v", body)
	}
}

func TestTypesInspectCommandRequiresProviderModelType(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Execute([]string{"types", "inspect", "--provider", "seeddance"}, &stdout, &stderr)

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

func TestTypesInspectCommandReturnsSchemaError(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"types", "inspect", "--provider", "seeddance", "--model", "v1", "--type", "missing"}, &stdout, &stderr, Options{ConfigDir: filepath.Join(t.TempDir(), "config")})

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
	if body.Error.Code != errdefs.CodeSchemaNotFound {
		t.Fatalf("expected %s, got %q", errdefs.CodeSchemaNotFound, body.Error.Code)
	}
}

func writeCommandSchema(t *testing.T, root string, provider string, model string, schemaType string) {
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
  "artifactKind": "video",
  "displayName": "SeedDance Image to Video",
  "input": {"properties": {"prompt": {"type": "string"}}}
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("expected schema write, got %v", err)
	}
}
