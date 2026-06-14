package commands

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/auth"
	"github.com/a754962942/pi-pro-cli/internal/config"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestAuthLoginCommandPromptsAndStoresToken(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != serverapi.AuthLogin {
			t.Fatalf("expected %s, got %s", serverapi.AuthLogin, r.URL.Path)
		}
		return commandJSONResponse(http.StatusOK, `{"authToken":"sk-pipro-testtoken","user":{"username":"user@example.com"}}`), nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"auth", "login"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		AuthPrompter: &commandPrompter{username: "user@example.com", password: "secret"},
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	var body struct {
		OK            bool   `json:"ok"`
		Authenticated bool   `json:"authenticated"`
		Username      string `json:"username"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || !body.Authenticated || body.Username != "user@example.com" {
		t.Fatalf("unexpected login output %#v", body)
	}
	cfg, err := config.Load(config.PathsFor(configDir))
	if err != nil {
		t.Fatalf("expected config load, got %v", err)
	}
	if cfg.AuthToken != "sk-pipro-testtoken" {
		t.Fatalf("expected token stored, got %#v", cfg)
	}
}

func TestAuthLoginCommandDoesNotAcceptPasswordFlag(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := Execute([]string{"auth", "login", "--password", "secret"}, &stdout, &stderr)

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

func TestAuthStatusCommandDoesNotExposeToken(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-secret", Username: "user@example.com"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"auth", "status"}, &stdout, &stderr, Options{ConfigDir: configDir})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	if strings.Contains(stdout.String(), "sk-pipro-secret") {
		t.Fatalf("expected status not to expose token: %s", stdout.String())
	}
	var body struct {
		OK            bool   `json:"ok"`
		Authenticated bool   `json:"authenticated"`
		Username      string `json:"username"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON output, got %v", err)
	}
	if !body.OK || !body.Authenticated || body.Username != "user@example.com" {
		t.Fatalf("unexpected status output %#v", body)
	}
}

func TestAuthLogoutCommandRemovesToken(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	paths := config.PathsFor(configDir)
	if err := config.Save(paths, config.File{AuthToken: "sk-pipro-secret", Username: "user@example.com"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"auth", "logout"}, &stdout, &stderr, Options{ConfigDir: configDir})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d with stdout %s", exitCode, stdout.String())
	}
	cfg, err := config.Load(paths)
	if err != nil {
		t.Fatalf("expected config load, got %v", err)
	}
	if cfg.AuthToken != "" || cfg.Username != "" {
		t.Fatalf("expected auth config removed, got %#v", cfg)
	}
}

type commandPrompter struct {
	username string
	password string
}

func (p *commandPrompter) PromptUsername() (string, error) {
	return p.username, nil
}

func (p *commandPrompter) PromptPassword() (string, error) {
	return p.password, nil
}

var _ auth.Prompter = (*commandPrompter)(nil)
