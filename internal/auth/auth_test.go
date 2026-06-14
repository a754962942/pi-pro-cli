package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/config"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestLoginPromptsUsernameBeforePasswordAndStoresToken(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	prompter := &fakePrompter{username: "user@example.com", password: "secret"}
	httpClient := authHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != serverapi.AuthLogin {
			t.Fatalf("expected %s, got %s", serverapi.AuthLogin, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("expected JSON request, got %v", err)
		}
		if body["username"] != "user@example.com" || body["password"] != "secret" {
			t.Fatalf("unexpected login body %#v", body)
		}
		return jsonResponse(http.StatusOK, `{"authToken":"sk-pipro-testtoken","user":{"username":"user@example.com"}}`), nil
	})

	result, err := Login(context.Background(), Options{
		ConfigDir:  configDir,
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
		Prompter:   prompter,
	})

	if err != nil {
		t.Fatalf("expected login to succeed, got %v", err)
	}
	if !result.OK || !result.Authenticated || result.Username != "user@example.com" {
		t.Fatalf("unexpected login result %#v", result)
	}
	if got := prompter.calls; len(got) != 2 || got[0] != "username" || got[1] != "password" {
		t.Fatalf("expected username prompt before password, got %#v", got)
	}
	cfg, err := config.Load(config.PathsFor(configDir))
	if err != nil {
		t.Fatalf("expected config load, got %v", err)
	}
	if cfg.AuthToken != "sk-pipro-testtoken" || cfg.Username != "user@example.com" {
		t.Fatalf("expected stored auth config, got %#v", cfg)
	}
}

func TestLoginRejectsEmptyCredentialsBeforeServerCall(t *testing.T) {
	for _, tt := range []struct {
		name     string
		username string
		password string
	}{
		{name: "username", username: "", password: "secret"},
		{name: "password", username: "user@example.com", password: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			_, err := Login(context.Background(), Options{
				ConfigDir: filepath.Join(t.TempDir(), "config"),
				ServerURL: "https://api.example.test",
				HTTPClient: authHTTPClient(func(r *http.Request) (*http.Response, error) {
					called = true
					return nil, nil
				}),
				Prompter: &fakePrompter{username: tt.username, password: tt.password},
			})

			var appErr apperror.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected AppError, got %T %[1]v", err)
			}
			if appErr.Code != errdefs.CodeAuthRequired {
				t.Fatalf("expected %s, got %q", errdefs.CodeAuthRequired, appErr.Code)
			}
			if called {
				t.Fatalf("expected empty credentials to fail before server call")
			}
		})
	}
}

func TestLoginRejectsMissingTokenResponse(t *testing.T) {
	_, err := Login(context.Background(), Options{
		ConfigDir: filepath.Join(t.TempDir(), "config"),
		ServerURL: "https://api.example.test",
		HTTPClient: authHTTPClient(func(r *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"user":{"username":"user@example.com"}}`), nil
		}),
		Prompter: &fakePrompter{username: "user@example.com", password: "secret"},
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeServerResponseInvalid {
		t.Fatalf("expected %s, got %q", errdefs.CodeServerResponseInvalid, appErr.Code)
	}
}

func TestStatusDoesNotExposeToken(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-secret", Username: "user@example.com"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}

	result, err := Status(Options{ConfigDir: configDir})

	if err != nil {
		t.Fatalf("expected status to succeed, got %v", err)
	}
	if !result.OK || !result.Authenticated || result.Username != "user@example.com" {
		t.Fatalf("unexpected status result %#v", result)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("expected status marshal, got %v", err)
	}
	if bytes.Contains(data, []byte("sk-pipro-secret")) {
		t.Fatalf("expected status not to expose token: %s", data)
	}
}

func TestLogoutRemovesStoredAuth(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	paths := config.PathsFor(configDir)
	if err := config.Save(paths, config.File{AuthToken: "sk-pipro-secret", Username: "user@example.com"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}

	result, err := Logout(Options{ConfigDir: configDir})

	if err != nil {
		t.Fatalf("expected logout to succeed, got %v", err)
	}
	if !result.OK || result.Authenticated {
		t.Fatalf("unexpected logout result %#v", result)
	}
	cfg, err := config.Load(paths)
	if err != nil {
		t.Fatalf("expected config load, got %v", err)
	}
	if cfg.AuthToken != "" || cfg.Username != "" {
		t.Fatalf("expected auth fields removed, got %#v", cfg)
	}
}

type fakePrompter struct {
	username string
	password string
	calls    []string
}

func (p *fakePrompter) PromptUsername() (string, error) {
	p.calls = append(p.calls, "username")
	return p.username, nil
}

func (p *fakePrompter) PromptPassword() (string, error) {
	p.calls = append(p.calls, "password")
	return p.password, nil
}

type authHTTPClient func(req *http.Request) (*http.Response, error)

func (f authHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
