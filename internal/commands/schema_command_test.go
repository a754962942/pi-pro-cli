package commands

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestSchemaBriefCommandUsesRemoteCapabilities(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CapabilityTypes:
			return commandJSONResponse(http.StatusOK, `{"types":[{"code":"image-to-video","name":"首帧生视频","artifactKind":"video"},{"code":"multi-image-to-video","name":"多图参考生视频","artifactKind":"video"}]}`), nil
		case serverapi.CapabilityModels("image-to-video"):
			return commandJSONResponse(http.StatusOK, `{"models":[{"code":"grok-video-1.5","name":"grok-video-1.5","modality":"video","supportedEventTypes":["image-to-video"],"providers":[{"providerCode":"grok","providerModelId":"grok-video-1.5","healthStatus":"healthy"}]}]}`), nil
		case serverapi.CapabilityModels("multi-image-to-video"):
			return commandJSONResponse(http.StatusOK, `{"models":[{"code":"Seedance-2.0","name":"Seedance-2.0","modality":"video","supportedEventTypes":["multi-image-to-video"],"providers":[{"providerCode":"seeddance","providerModelId":"Seedance-2.0","healthStatus":"healthy"}]}]}`), nil
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
			return nil, nil
		}
	})

	exitCode := ExecuteWithOptions([]string{"schema", "--brief"}, &stdout, &stderr, Options{
		ConfigDir:  filepath.Join(t.TempDir(), "config"),
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "apiKey") || strings.Contains(stdout.String(), "baseUrl") {
		t.Fatalf("expected schema brief without provider secrets, got %s", stdout.String())
	}
	var body struct {
		OK           bool   `json:"ok"`
		SchemaSource string `json:"schemaSource"`
		Types        []struct {
			Type   string `json:"type"`
			Models []struct {
				Code      string `json:"code"`
				Providers []struct {
					ProviderCode string `json:"providerCode"`
				} `json:"providers"`
			} `json:"models"`
		} `json:"types"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || body.SchemaSource != "remote" || len(body.Types) != 2 ||
		body.Types[0].Type != "image-to-video" ||
		body.Types[0].Models[0].Code != "grok-video-1.5" ||
		body.Types[1].Models[0].Providers[0].ProviderCode != "seeddance" {
		t.Fatalf("unexpected schema brief output %#v", body)
	}
}

func TestSchemaInspectCommandUsesRemoteSchema(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != serverapi.CLISchema {
			t.Fatalf("expected schema request, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("provider") != "vidu" ||
			r.URL.Query().Get("model") != "vidu/viduq3-turbo_img2video" ||
			r.URL.Query().Get("type") != "image-to-video" {
			t.Fatalf("unexpected schema query %s", r.URL.RawQuery)
		}
		return commandJSONResponse(http.StatusOK, `{"ok":true,"schema":{"provider":"vidu","model":"vidu/viduq3-turbo_img2video","type":"image-to-video","schemaVersion":"1.0","artifactKind":"video","input":{"properties":{"prompt":{"type":"string"}}}}}`), nil
	})

	exitCode := ExecuteWithOptions([]string{"schema", "inspect", "--provider", "vidu", "--model", "vidu/viduq3-turbo_img2video", "--type", "image-to-video"}, &stdout, &stderr, Options{
		ConfigDir:  filepath.Join(t.TempDir(), "config"),
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	var body struct {
		OK     bool `json:"ok"`
		Schema struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Type     string `json:"type"`
		} `json:"schema"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || body.Schema.Provider != "vidu" || body.Schema.Model != "vidu/viduq3-turbo_img2video" {
		t.Fatalf("unexpected schema inspect output %#v", body)
	}
}
