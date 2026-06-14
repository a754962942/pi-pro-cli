package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/config"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

func TestTypesListCommandReturnsLocalSchemas(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	writeCommandSchema(t, config.PathsFor(configDir).SchemasDir, "seeddance", "v1", "image-to-video")
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
	if !body.OK || len(body.Types) != 1 {
		t.Fatalf("unexpected list output %#v", body)
	}
	if body.Types[0].Provider != "seeddance" || body.Types[0].Model != "v1" || body.Types[0].Type != "image-to-video" || body.Types[0].ArtifactKind != "video" {
		t.Fatalf("unexpected type summary %#v", body.Types[0])
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
