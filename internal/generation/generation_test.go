package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/assets"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/schema"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestRunNoWaitSubmitsNormalizedRequestAndDoesNotPoll(t *testing.T) {
	registry := fakeRegistry{selected: generationTestSchema("video")}
	inputPath := writeGenerationInput(t, map[string]any{"prompt": "lake"})
	var received map[string]any
	httpClient := generationHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != serverapi.Generations {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("expected generation JSON request, got %v", err)
		}
		return generationJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
	})

	result, err := NewService(Options{
		Registry:   registry,
		ServerURL:  "https://api.example.test",
		AuthToken:  "sk-pipro-test",
		HTTPClient: httpClient,
		Stdin:      bytes.NewReader(nil),
	}).Run(context.Background(), Request{
		Command:       "generateVideo",
		ArtifactKind:  "video",
		Provider:      "mock-provider",
		Model:         "mock-model",
		Type:          "text-to-video",
		InputPath:     inputPath,
		Wait:          false,
		WaitSpecified: true,
	})

	if err != nil {
		t.Fatalf("expected no-wait generation, got %v", err)
	}
	if !result.OK || result.Status != "submitted" || result.JobID != "job_123" {
		t.Fatalf("unexpected result %#v", result)
	}
	if received["provider"] != "mock-provider" || received["model"] != "mock-model" || received["type"] != "text-to-video" || received["artifactKind"] != "video" {
		t.Fatalf("unexpected request metadata %#v", received)
	}
	input, ok := received["input"].(map[string]any)
	if !ok {
		t.Fatalf("expected input object, got %#v", received["input"])
	}
	if input["prompt"] != "lake" || input["duration"] != float64(5) {
		t.Fatalf("expected normalized input with default, got %#v", input)
	}
}

func TestRunWaitPollsTaskUntilSuccess(t *testing.T) {
	calls := 0
	httpClient := generationHTTPClient(func(r *http.Request) (*http.Response, error) {
		calls++
		switch {
		case r.Method == http.MethodPost && r.URL.Path == serverapi.Generations:
			return generationJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
		case r.Method == http.MethodGet && r.URL.Path == serverapi.TaskStatus("job_123") && calls == 2:
			return generationJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"running"}`), nil
		case r.Method == http.MethodGet && r.URL.Path == serverapi.TaskStatus("job_123"):
			return generationJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"succeeded","artifacts":[{"url":"https://server.example/video.mp4","mime":"video/mp4","kind":"video"}]}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})
	var stderr bytes.Buffer

	result, err := NewService(Options{
		Registry:   fakeRegistry{selected: generationTestSchema("video")},
		ServerURL:  "https://api.example.test",
		AuthToken:  "sk-pipro-test",
		HTTPClient: httpClient,
		Stderr:     &stderr,
		TaskSleep:  func(delay time.Duration) {},
		TaskJitter: func(delay time.Duration) time.Duration { return delay },
	}).Run(context.Background(), Request{
		Command:      "generateVideo",
		ArtifactKind: "video",
		Provider:     "mock-provider",
		Model:        "mock-model",
		Type:         "text-to-video",
		CLIValues:    map[string]any{"prompt": "lake"},
		Wait:         true,
		Timeout:      time.Minute,
		PollInterval: time.Second,
		PollMax:      time.Second,
		PollBackoff:  1,
	})

	if err != nil {
		t.Fatalf("expected waited generation, got %v", err)
	}
	if !result.OK || result.Status != "succeeded" || len(result.Artifacts) != 1 {
		t.Fatalf("unexpected waited result %#v", result)
	}
	if stderr.String() == "" {
		t.Fatalf("expected polling diagnostics on stderr")
	}
}

func TestRunRejectsMissingAuthBeforeServerCall(t *testing.T) {
	called := false

	_, err := NewService(Options{
		Registry: fakeRegistry{selected: generationTestSchema("video")},
		HTTPClient: generationHTTPClient(func(r *http.Request) (*http.Response, error) {
			called = true
			return nil, nil
		}),
	}).Run(context.Background(), Request{
		Command:      "generateVideo",
		ArtifactKind: "video",
		Provider:     "mock-provider",
		Model:        "mock-model",
		Type:         "text-to-video",
		CLIValues:    map[string]any{"prompt": "lake"},
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeAuthRequired {
		t.Fatalf("expected %s, got %q", errdefs.CodeAuthRequired, appErr.Code)
	}
	if called {
		t.Fatalf("expected auth failure before server call")
	}
}

func TestRunRejectsInvalidSchemaBeforeServerCall(t *testing.T) {
	called := false

	_, err := NewService(Options{
		Registry:  fakeRegistry{err: apperror.AppError{Code: errdefs.CodeSchemaInvalid, Message: "schema invalid", Kind: apperror.KindValidation}},
		AuthToken: "sk-pipro-test",
		HTTPClient: generationHTTPClient(func(r *http.Request) (*http.Response, error) {
			called = true
			return nil, nil
		}),
	}).Run(context.Background(), Request{
		Command:      "generateVideo",
		ArtifactKind: "video",
		Provider:     "mock-provider",
		Model:        "mock-model",
		Type:         "text-to-video",
		CLIValues:    map[string]any{"prompt": "lake"},
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeSchemaInvalid {
		t.Fatalf("expected %s, got %q", errdefs.CodeSchemaInvalid, appErr.Code)
	}
	if called {
		t.Fatalf("expected schema failure before server call")
	}
}

func TestRunReturnsTaskErrorWhenSubmitResponseIsFailed(t *testing.T) {
	httpClient := generationHTTPClient(func(r *http.Request) (*http.Response, error) {
		return generationJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"failed","error":{"code":"PROVIDER_REJECTED","message":"provider rejected request"}}`), nil
	})

	_, err := NewService(Options{
		Registry:   fakeRegistry{selected: generationTestSchema("video")},
		ServerURL:  "https://api.example.test",
		AuthToken:  "sk-pipro-test",
		HTTPClient: httpClient,
	}).Run(context.Background(), Request{
		Command:      "generateVideo",
		ArtifactKind: "video",
		Provider:     "mock-provider",
		Model:        "mock-model",
		Type:         "text-to-video",
		CLIValues:    map[string]any{"prompt": "lake"},
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeTaskFailed {
		t.Fatalf("expected %s, got %q", errdefs.CodeTaskFailed, appErr.Code)
	}
}

func TestRunResolvesFileFieldsThroughAssetDB(t *testing.T) {
	store := openGenerationAssetStore(t)
	filePath := writeGenerationFile(t, "image.png", []byte("image"))
	if _, err := store.RecordDownloaded(assets.Record{ServerAssetID: "asset_123", SourceURL: "https://server.example/image.png", MIME: "image/png", FilePath: filePath}); err != nil {
		t.Fatalf("expected asset record, got %v", err)
	}
	var received map[string]any
	httpClient := generationHTTPClient(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("expected request body, got %v", err)
		}
		return generationJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
	})

	_, err := NewService(Options{
		Registry:      fakeRegistry{selected: generationImageToVideoSchema()},
		ServerURL:     "https://api.example.test",
		AuthToken:     "sk-pipro-test",
		HTTPClient:    httpClient,
		AssetResolver: assets.NewResolver(store, nil),
	}).Run(context.Background(), Request{
		Command:       "generateVideo",
		ArtifactKind:  "video",
		Provider:      "mock-provider",
		Model:         "mock-model",
		Type:          "image-to-video",
		CLIValues:     map[string]any{"prompt": "lake", "image": filePath},
		Wait:          false,
		WaitSpecified: true,
	})

	if err != nil {
		t.Fatalf("expected asset-db generation, got %v", err)
	}
	input := received["input"].(map[string]any)
	image := input["image"].(map[string]any)
	if image["url"] != "https://server.example/image.png" || image["source"] != "asset-db" {
		t.Fatalf("expected file resolved to asset reference, got %#v", image)
	}
}

type fakeRegistry struct {
	selected schema.Schema
	err      error
}

func (r fakeRegistry) Get(provider string, model string, schemaType string) (schema.Schema, error) {
	if r.err != nil {
		return schema.Schema{}, r.err
	}
	return r.selected, nil
}

func (r fakeRegistry) List() ([]schema.Summary, error) {
	return nil, nil
}

type generationHTTPClient func(req *http.Request) (*http.Response, error)

func (f generationHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func generationJSONResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func generationTestSchema(artifactKind string) schema.Schema {
	return schema.Schema{
		Provider:      "mock-provider",
		Model:         "mock-model",
		Type:          "text-to-video",
		SchemaVersion: "1.0",
		ArtifactKind:  artifactKind,
		Input: map[string]any{
			"required": []any{"prompt"},
			"properties": map[string]any{
				"prompt": map[string]any{"type": "string", "minLength": float64(1)},
				"duration": map[string]any{
					"type":    "number",
					"default": float64(5),
				},
			},
		},
	}
}

func generationImageToVideoSchema() schema.Schema {
	selected := generationTestSchema("video")
	selected.Type = "image-to-video"
	selected.Input = map[string]any{
		"required": []any{"prompt", "image"},
		"properties": map[string]any{
			"prompt": map[string]any{"type": "string"},
			"image": map[string]any{
				"type":        "file",
				"fileResolve": "asset-db",
			},
		},
	}
	return selected
}

func writeGenerationInput(t *testing.T, value map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.json")
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("expected marshal, got %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("expected input write, got %v", err)
	}
	return path
}

func openGenerationAssetStore(t *testing.T) *assets.Store {
	t.Helper()
	store, err := assets.Open(filepath.Join(t.TempDir(), "assets.sqlite"))
	if err != nil {
		t.Fatalf("expected asset store open, got %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func writeGenerationFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("expected file write, got %v", err)
	}
	return path
}
