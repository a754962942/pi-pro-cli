package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestDoJSONSendsRequestAndDecodesResponse(t *testing.T) {
	httpClient := fakeHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != serverapi.AuthLogin {
			t.Errorf("expected %s, got %s", serverapi.AuthLogin, r.URL.Path)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("expected JSON request, got %v", err)
		}
		if body["username"] != "user@example.com" {
			t.Errorf("expected username, got %#v", body)
		}

		return jsonResponse(http.StatusOK, `{"authToken":"sk-pipro-testtoken"}`), nil
	})

	client := New(Config{ServerURL: "https://api.example.test", HTTPClient: httpClient})

	var response struct {
		AuthToken string `json:"authToken"`
	}
	err := client.DoJSON(context.Background(), http.MethodPost, serverapi.AuthLogin, map[string]string{
		"username": "user@example.com",
	}, &response)

	if err != nil {
		t.Fatalf("expected request to succeed, got %v", err)
	}
	if response.AuthToken != "sk-pipro-testtoken" {
		t.Fatalf("expected decoded token, got %q", response.AuthToken)
	}
}

func TestDoJSONInjectsBearerToken(t *testing.T) {
	httpClient := fakeHTTPClient(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-pipro-testtoken" {
			t.Errorf("expected bearer token, got %q", got)
		}
		return jsonResponse(http.StatusOK, `{"ok":true}`), nil
	})

	client := New(Config{
		ServerURL:  "https://api.example.test",
		AuthToken:  "sk-pipro-testtoken",
		HTTPClient: httpClient,
	})

	var response map[string]any
	if err := client.DoJSON(context.Background(), http.MethodGet, serverapi.TaskStatus("job_123"), nil, &response); err != nil {
		t.Fatalf("expected request to succeed, got %v", err)
	}
}

func TestDoJSONMapsAuthFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			httpClient := fakeHTTPClient(func(r *http.Request) (*http.Response, error) {
				return jsonResponse(status, `{"error":{"code":"TOKEN_REJECTED","message":"token rejected"}}`), nil
			})

			client := New(Config{ServerURL: "https://api.example.test", HTTPClient: httpClient})

			var response map[string]any
			err := client.DoJSON(context.Background(), http.MethodGet, serverapi.TaskStatus("job_123"), nil, &response)

			var appErr apperror.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected AppError, got %T %[1]v", err)
			}
			if appErr.Kind != apperror.KindAuth {
				t.Fatalf("expected auth kind, got %q", appErr.Kind)
			}
			if appErr.Code != errdefs.CodeAuthInvalid {
				t.Fatalf("expected %s, got %q", errdefs.CodeAuthInvalid, appErr.Code)
			}
			details := appErr.Details.(map[string]any)
			if details["serverCode"] != "TOKEN_REJECTED" {
				t.Fatalf("expected serverCode to be preserved, got %#v", details)
			}
			if details["serverMessage"] != "token rejected" {
				t.Fatalf("expected serverMessage to be preserved, got %#v", details)
			}
		})
	}
}

func TestDoJSONPreservesServerErrors(t *testing.T) {
	httpClient := fakeHTTPClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, `{"error":{"code":"PROVIDER_BAD_REQUEST","message":"bad provider request"}}`), nil
	})

	client := New(Config{ServerURL: "https://api.example.test", HTTPClient: httpClient})

	var response map[string]any
	err := client.DoJSON(context.Background(), http.MethodPost, serverapi.Generations, map[string]string{"type": "x"}, &response)

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Kind != apperror.KindNetwork {
		t.Fatalf("expected network kind, got %q", appErr.Kind)
	}
	if appErr.Code != errdefs.CodeServerRequestFailed {
		t.Fatalf("expected %s, got %q", errdefs.CodeServerRequestFailed, appErr.Code)
	}
	details := appErr.Details.(map[string]any)
	if details["serverCode"] != "PROVIDER_BAD_REQUEST" {
		t.Fatalf("expected serverCode to be preserved, got %#v", details)
	}
	if details["statusCode"] != float64(http.StatusBadRequest) {
		t.Fatalf("expected statusCode to be preserved, got %#v", details)
	}
}

func TestDoJSONMapsInvalidServerResponse(t *testing.T) {
	httpClient := fakeHTTPClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{`), nil
	})

	client := New(Config{ServerURL: "https://api.example.test", HTTPClient: httpClient})

	var response map[string]any
	err := client.DoJSON(context.Background(), http.MethodGet, serverapi.TaskStatus("job_123"), nil, &response)

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeServerResponseInvalid {
		t.Fatalf("expected %s, got %q", errdefs.CodeServerResponseInvalid, appErr.Code)
	}
}

func TestDoJSONMapsNetworkErrors(t *testing.T) {
	httpClient := fakeHTTPClient(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})

	client := New(Config{ServerURL: "https://api.example.test", HTTPClient: httpClient})

	var response map[string]any
	err := client.DoJSON(context.Background(), http.MethodGet, serverapi.TaskStatus("job_123"), nil, &response)

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Kind != apperror.KindNetwork {
		t.Fatalf("expected network kind, got %q", appErr.Kind)
	}
	if appErr.Code != errdefs.CodeNetworkRequestFailed {
		t.Fatalf("expected %s, got %q", errdefs.CodeNetworkRequestFailed, appErr.Code)
	}
}

func TestRetryableStatusClassifier(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable} {
		if !IsRetryableStatus(status) {
			t.Fatalf("expected status %d to be retryable", status)
		}
	}

	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		if IsRetryableStatus(status) {
			t.Fatalf("expected status %d to be non-retryable", status)
		}
	}
}

type fakeHTTPClient func(req *http.Request) (*http.Response, error)

func (f fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
