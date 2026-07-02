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
	"github.com/a754962942/pi-pro-cli/internal/client"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/schema"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
	"github.com/a754962942/pi-pro-cli/internal/validation"
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

func TestRunWaitDownloadsSingleArtifactToOutput(t *testing.T) {
	store := openGenerationAssetStore(t)
	outputPath := filepath.Join(t.TempDir(), "result.mp4")
	calls := 0
	httpClient := generationHTTPClient(func(r *http.Request) (*http.Response, error) {
		calls++
		switch {
		case r.Method == http.MethodPost && r.URL.Path == serverapi.Generations:
			return generationJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
		case r.Method == http.MethodGet && r.URL.Path == serverapi.TaskStatus("job_123"):
			return generationJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"succeeded","artifacts":[{"url":"https://server.example/video.mp4","mime":"video/mp4","kind":"video"}]}`), nil
		case r.Method == http.MethodGet && r.URL.String() == "https://server.example/video.mp4":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(bytes.NewBufferString("video-bytes"))}, nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
			return nil, nil
		}
	})

	result, err := NewService(Options{
		Registry:   fakeRegistry{selected: generationTestSchema("video")},
		ServerURL:  "https://api.example.test",
		AuthToken:  "sk-pipro-test",
		HTTPClient: httpClient,
		AssetStore: store,
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
		OutputPath:   outputPath,
		Timeout:      time.Minute,
		PollInterval: time.Second,
		PollMax:      time.Second,
		PollBackoff:  1,
	})

	if err != nil {
		t.Fatalf("expected downloaded artifact, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected submit, poll, and download calls, got %d", calls)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil || string(data) != "video-bytes" {
		t.Fatalf("expected downloaded file, data=%q err=%v", string(data), err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Path != outputPath {
		t.Fatalf("expected result artifact path, got %#v", result.Artifacts)
	}
	asset, ok, err := store.FindByPath(outputPath)
	if err != nil || !ok || asset.JobID != "job_123" || asset.Provider != "mock-provider" || asset.ArtifactKind != "video" {
		t.Fatalf("expected downloaded asset record, ok=%v asset=%#v err=%v", ok, asset, err)
	}
}

func TestRunWaitDownloadsMultipleArtifactsToOutputDir(t *testing.T) {
	store := openGenerationAssetStore(t)
	outputDir := t.TempDir()
	httpClient := generationHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == serverapi.Generations:
			return generationJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
		case r.Method == http.MethodGet && r.URL.Path == serverapi.TaskStatus("job_123"):
			return generationJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"succeeded","artifacts":[{"url":"https://server.example/first.png","mime":"image/png","kind":"image"},{"url":"https://server.example/second.png","mime":"image/png","kind":"image"}]}`), nil
		case r.Method == http.MethodGet && r.URL.String() == "https://server.example/first.png":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewBufferString("first"))}, nil
		case r.Method == http.MethodGet && r.URL.String() == "https://server.example/second.png":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewBufferString("second"))}, nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
			return nil, nil
		}
	})

	result, err := NewService(Options{
		Registry:   fakeRegistry{selected: generationTestSchema("image")},
		ServerURL:  "https://api.example.test",
		AuthToken:  "sk-pipro-test",
		HTTPClient: httpClient,
		AssetStore: store,
		TaskSleep:  func(delay time.Duration) {},
		TaskJitter: func(delay time.Duration) time.Duration { return delay },
	}).Run(context.Background(), Request{
		Command:      "generateImage",
		ArtifactKind: "image",
		Provider:     "mock-provider",
		Model:        "mock-model",
		Type:         "text-to-video",
		CLIValues:    map[string]any{"prompt": "lake"},
		Wait:         true,
		OutputDir:    outputDir,
		Timeout:      time.Minute,
		PollInterval: time.Second,
		PollMax:      time.Second,
		PollBackoff:  1,
	})

	if err != nil {
		t.Fatalf("expected downloaded artifacts, got %v", err)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("expected two artifacts, got %#v", result.Artifacts)
	}
	for i, artifact := range result.Artifacts {
		if artifact.Path == "" || filepath.Dir(artifact.Path) != outputDir {
			t.Fatalf("expected artifact %d in output dir, got %#v", i, artifact)
		}
		if _, err := os.Stat(artifact.Path); err != nil {
			t.Fatalf("expected downloaded artifact file, got %v", err)
		}
	}
}

func TestRunRejectsOutputPathWithoutWait(t *testing.T) {
	_, err := NewService(Options{Registry: fakeRegistry{selected: generationTestSchema("video")}}).Run(context.Background(), Request{
		Command:      "generateVideo",
		ArtifactKind: "video",
		Provider:     "mock-provider",
		Model:        "mock-model",
		Type:         "text-to-video",
		CLIValues:    map[string]any{"prompt": "lake"},
		Wait:         false,
		OutputPath:   filepath.Join(t.TempDir(), "result.mp4"),
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeUsage {
		t.Fatalf("expected %s, got %q", errdefs.CodeUsage, appErr.Code)
	}
}

func TestRunRejectsExistingOutputPathBeforeSubmit(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "result.mp4")
	if err := os.WriteFile(outputPath, []byte("exists"), 0o600); err != nil {
		t.Fatalf("expected fixture file, got %v", err)
	}
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
		Wait:         true,
		OutputPath:   outputPath,
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeOutputPathExists {
		t.Fatalf("expected %s, got %q", errdefs.CodeOutputPathExists, appErr.Code)
	}
	if called {
		t.Fatalf("expected existing output path to fail before submit")
	}
}

func TestRunDryRunReturnsNormalizedRequestWithoutAuthOrServerCall(t *testing.T) {
	called := false

	result, err := NewService(Options{
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
		DryRun:       true,
	})

	if err != nil {
		t.Fatalf("expected dry-run result, got %v", err)
	}
	if called {
		t.Fatalf("expected dry-run to avoid server calls")
	}
	if !result.OK || result.Status != "dry-run" || !result.DryRun || result.Plan == nil || result.Plan.Submit || result.Plan.Wait || result.Plan.ResolveAssets {
		t.Fatalf("unexpected dry-run result %#v", result)
	}
	request, ok := result.Request.(validation.Request)
	if !ok {
		t.Fatalf("expected normalized validation request, got %T %#v", result.Request, result.Request)
	}
	if request.Provider != "mock-provider" ||
		request.Model != "mock-model" ||
		request.Type != "text-to-video" ||
		request.ArtifactKind != "video" ||
		request.Input["prompt"] != "lake" ||
		request.Input["duration"] != float64(5) {
		t.Fatalf("unexpected normalized request %#v", request)
	}
}

func TestRunDryRunDoesNotResolveFileFields(t *testing.T) {
	store := openGenerationAssetStore(t)
	filePath := writeGenerationFile(t, "image.png", []byte("image"))

	result, err := NewService(Options{
		Registry:      fakeRegistry{selected: generationImageToVideoSchema()},
		AssetResolver: assets.NewResolver(store, nil),
	}).Run(context.Background(), Request{
		Command:      "generateVideo",
		ArtifactKind: "video",
		Provider:     "mock-provider",
		Model:        "mock-model",
		Type:         "image-to-video",
		CLIValues:    map[string]any{"prompt": "lake", "image": filePath},
		DryRun:       true,
	})

	if err != nil {
		t.Fatalf("expected dry-run file preview, got %v", err)
	}
	request := result.Request.(validation.Request)
	if request.Input["image"] != filePath {
		t.Fatalf("expected dry-run to preserve local file path, got %#v", request.Input["image"])
	}
}

func TestRunRejectsUnsupportedCapabilityBeforeServerCall(t *testing.T) {
	called := false

	_, err := NewService(Options{
		Registry:     fakeRegistry{selected: generationTestSchema("video")},
		Capabilities: fakeCapabilityClient{models: client.CapabilityModelsResponse{Models: []client.CapabilityModel{}}},
		AuthToken:    "sk-pipro-test",
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
	if appErr.Code != errdefs.CodeCapabilityUnsupported {
		t.Fatalf("expected %s, got %q", errdefs.CodeCapabilityUnsupported, appErr.Code)
	}
	if called {
		t.Fatalf("expected capability failure before server call")
	}
}

func TestRunAcceptsAdvertisedCapability(t *testing.T) {
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

	_, err := NewService(Options{
		Registry:     fakeRegistry{selected: generationTestSchema("video")},
		Capabilities: fakeCapabilityClient{models: advertisedCapability("mock-provider", "mock-model", "text-to-video")},
		ServerURL:    "https://api.example.test",
		AuthToken:    "sk-pipro-test",
		HTTPClient:   httpClient,
	}).Run(context.Background(), Request{
		Command:      "generateVideo",
		ArtifactKind: "video",
		Provider:     "mock-provider",
		Model:        "mock-model",
		Type:         "text-to-video",
		CLIValues:    map[string]any{"prompt": "lake"},
		Wait:         false,
	})

	if err != nil {
		t.Fatalf("expected advertised capability generation, got %v", err)
	}
	if received["provider"] != "mock-provider" {
		t.Fatalf("expected generation request after capability validation, got %#v", received)
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

type fakeCapabilityClient struct {
	models client.CapabilityModelsResponse
	err    error
}

func (c fakeCapabilityClient) CapabilityModels(ctx context.Context, eventType string) (client.CapabilityModelsResponse, error) {
	if c.err != nil {
		return client.CapabilityModelsResponse{}, c.err
	}
	return c.models, nil
}

func advertisedCapability(provider string, model string, eventType string) client.CapabilityModelsResponse {
	return client.CapabilityModelsResponse{
		Models: []client.CapabilityModel{{
			Code:                model,
			SupportedEventTypes: []string{eventType},
			Providers: []client.CapabilityProviderMapping{{
				ProviderCode: provider,
			}},
		}},
	}
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
