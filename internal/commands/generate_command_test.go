package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/a754962942/pi-pro-cli/internal/assets"
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
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CLISchema {
			assertSchemaQuery(t, r, "mock-provider", "mock-model", "text-to-video")
			return commandJSONResponse(http.StatusOK, remoteSchemaBody("mock-provider", "mock-model", "text-to-video", "video")), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CapabilityModels("text-to-video") {
			return commandJSONResponse(http.StatusOK, remoteCapabilityModelsBody("mock-provider", "mock-model", "text-to-video")), nil
		}
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

func TestGenerateVideoCommandSubmitsTwoImageToVideo(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-test", Username: "user@example.com"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	writeTwoImageGenerateCommandSchema(t, config.PathsFor(configDir).SchemasDir, "mock-provider", "mock-model")
	var received map[string]any
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CLISchema {
			assertSchemaQuery(t, r, "mock-provider", "mock-model", "two-image-to-video")
			return commandJSONResponse(http.StatusOK, remoteSchemaBody("mock-provider", "mock-model", "two-image-to-video", "video")), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CapabilityModels("two-image-to-video") {
			return commandJSONResponse(http.StatusOK, remoteCapabilityModelsBody("mock-provider", "mock-model", "two-image-to-video")), nil
		}
		if r.Method != http.MethodPost || r.URL.Path != serverapi.Generations {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("expected generation request body, got %v", err)
		}
		return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateVideo", "--provider", "mock-provider", "--model", "mock-model", "--type", "two-image-to-video", "--input", "-", "--no-wait"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		CommandStdin: strings.NewReader(`{"prompt":"walk forward","image":["data:image/jpeg;base64,start","data:image/jpeg;base64,end"]}`),
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	input, _ := received["input"].(map[string]any)
	images, _ := input["image"].([]any)
	if received["type"] != "two-image-to-video" || len(images) != 2 {
		t.Fatalf("expected two-image-to-video request with two images, got %#v", received)
	}
}

func TestGenerateVideoCommandUsesRemoteSchemaForSlashModel(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-test"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	var received map[string]any
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CLISchema {
			assertSchemaQuery(t, r, "vidu", "vidu/viduq3-turbo_img2video", "image-to-video")
			return commandJSONResponse(http.StatusOK, remoteSchemaBody("vidu", "vidu/viduq3-turbo_img2video", "image-to-video", "video")), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CapabilityModels("image-to-video") {
			return commandJSONResponse(http.StatusOK, remoteCapabilityModelsBody("vidu", "vidu/viduq3-turbo_img2video", "image-to-video")), nil
		}
		if r.Method != http.MethodPost || r.URL.Path != serverapi.Generations {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("expected generation request body, got %v", err)
		}
		return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateVideo", "--provider", "vidu", "--model", "vidu/viduq3-turbo_img2video", "--type", "image-to-video", "--input", "-", "--no-wait"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		CommandStdin: strings.NewReader(`{"prompt":"lake","image":["data:image/jpeg;base64,start"]}`),
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if received["provider"] != "vidu" || received["model"] != "vidu/viduq3-turbo_img2video" || received["type"] != "image-to-video" {
		t.Fatalf("expected remote schema backed submit, got %#v", received)
	}
}

func TestGenerateImageCommandSubmitsTextToImage(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-test"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	var received map[string]any
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CLISchema {
			assertSchemaQuery(t, r, "qiling", "gpt-image-2", "text-to-image")
			return commandJSONResponse(http.StatusOK, remoteSchemaBody("qiling", "gpt-image-2", "text-to-image", "image")), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CapabilityModels("text-to-image") {
			return commandJSONResponse(http.StatusOK, remoteCapabilityModelsBody("qiling", "gpt-image-2", "text-to-image")), nil
		}
		if r.Method != http.MethodPost || r.URL.Path != serverapi.Generations {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("expected generation request body, got %v", err)
		}
		return commandJSONResponse(http.StatusOK, `{"jobId":"job_img_123","status":"queued"}`), nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateImage", "--provider", "qiling", "--model", "gpt-image-2", "--type", "text-to-image", "--input", "-", "--no-wait"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		CommandStdin: strings.NewReader(`{"prompt":"a cinematic product photo"}`),
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	input, _ := received["input"].(map[string]any)
	if received["provider"] != "qiling" || received["model"] != "gpt-image-2" || received["type"] != "text-to-image" || received["artifactKind"] != "image" || input["prompt"] != "a cinematic product photo" {
		t.Fatalf("expected text-to-image request, got %#v", received)
	}
}

func TestGenerateImageCommandSubmitsImageToImageURLs(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-test"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	var received map[string]any
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CLISchema {
			assertSchemaQuery(t, r, "qiling", "nano-banana-2", "image-to-image")
			return commandJSONResponse(http.StatusOK, remoteImageToImageSchemaBody("qiling", "nano-banana-2")), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CapabilityModels("image-to-image") {
			return commandJSONResponse(http.StatusOK, remoteCapabilityModelsBody("qiling", "nano-banana-2", "image-to-image")), nil
		}
		if r.Method != http.MethodPost || r.URL.Path != serverapi.Generations {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("expected generation request body, got %v", err)
		}
		return commandJSONResponse(http.StatusOK, `{"jobId":"job_img_456","status":"queued"}`), nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateImage", "--provider", "qiling", "--model", "nano-banana-2", "--type", "image-to-image", "--input", "-", "--no-wait"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		CommandStdin: strings.NewReader(`{"prompt":"turn the subject into watercolor","urls":["data:image/png;base64,abc","https://example.test/ref.png"]}`),
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	input, _ := received["input"].(map[string]any)
	urls, _ := input["urls"].([]any)
	if received["type"] != "image-to-image" || received["artifactKind"] != "image" || len(urls) != 2 || urls[0] != "data:image/png;base64,abc" || urls[1] != "https://example.test/ref.png" {
		t.Fatalf("expected image-to-image request with urls, got %#v", received)
	}
}

func TestGenerateVideoCommandUploadsLocalImageBeforeSubmit(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-test"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	imagePath := filepath.Join(t.TempDir(), "frame.png")
	if err := os.WriteFile(imagePath, []byte("image-bytes"), 0o600); err != nil {
		t.Fatalf("expected image write, got %v", err)
	}
	var received map[string]any
	uploaded := false
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == serverapi.CLISchema:
			assertSchemaQuery(t, r, "vidu", "vidu/viduq3-turbo_img2video", "image-to-video")
			return commandJSONResponse(http.StatusOK, remoteURIArraySchemaBody("vidu", "vidu/viduq3-turbo_img2video", "image-to-video", "video", "images")), nil
		case r.Method == http.MethodGet && r.URL.Path == serverapi.CapabilityModels("image-to-video"):
			return commandJSONResponse(http.StatusOK, remoteCapabilityModelsBody("vidu", "vidu/viduq3-turbo_img2video", "image-to-video")), nil
		case r.Method == http.MethodPost && r.URL.Path == serverapi.AssetsUpload:
			if got := r.Header.Get("Authorization"); got != "Bearer sk-pipro-test" {
				t.Fatalf("expected upload bearer token, got %q", got)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("expected multipart file, got %v", err)
			}
			defer file.Close()
			body, _ := io.ReadAll(file)
			if header.Filename != "frame.png" || string(body) != "image-bytes" {
				t.Fatalf("unexpected uploaded file %q %q", header.Filename, string(body))
			}
			uploaded = true
			return commandJSONResponse(http.StatusCreated, `{"assetId":"asset_uploaded","url":"https://server.example/uploads/frame.png","mime":"image/png","sizeBytes":11,"sha256":"abc"}`), nil
		case r.Method == http.MethodPost && r.URL.Path == serverapi.Generations:
			if !uploaded {
				t.Fatalf("expected upload before generation submit")
			}
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("expected generation request body, got %v", err)
			}
			return commandJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateVideo", "--provider", "vidu", "--model", "vidu/viduq3-turbo_img2video", "--type", "image-to-video", "--input", "-", "--no-wait"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		CommandStdin: strings.NewReader(`{"prompt":"lake","images":["` + imagePath + `"]}`),
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	input := received["input"].(map[string]any)
	images := input["images"].([]any)
	image := images[0].(map[string]any)
	if image["url"] != "https://server.example/uploads/frame.png" || image["source"] != "upload" {
		t.Fatalf("expected uploaded image reference, got %#v", image)
	}
	store, err := assets.Open(config.PathsFor(configDir).AssetDB)
	if err != nil {
		t.Fatalf("expected asset store open, got %v", err)
	}
	defer store.Close()
	asset, ok, err := store.FindByPath(imagePath)
	if err != nil || !ok || asset.ServerAssetID != "asset_uploaded" {
		t.Fatalf("expected uploaded asset recorded, ok=%v asset=%#v err=%v", ok, asset, err)
	}
}

func TestGenerateImageCommandDryRunPrintsNormalizedRequestWithoutSubmitting(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CLISchema {
			assertSchemaQuery(t, r, "qiling", "gpt-image-2", "text-to-image")
			return commandJSONResponse(http.StatusOK, remoteSchemaBody("qiling", "gpt-image-2", "text-to-image", "image")), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CapabilityModels("text-to-image") {
			return commandJSONResponse(http.StatusOK, remoteCapabilityModelsBody("qiling", "gpt-image-2", "text-to-image")), nil
		}
		t.Fatalf("dry-run should not submit or poll, got %s %s", r.Method, r.URL.Path)
		return nil, nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateImage", "--provider", "qiling", "--model", "gpt-image-2", "--type", "text-to-image", "--input", "-", "--dry-run"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		CommandStdin: strings.NewReader(`{"prompt":"a cinematic product photo"}`),
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	var body struct {
		OK      bool   `json:"ok"`
		Status  string `json:"status"`
		DryRun  bool   `json:"dryRun"`
		Request struct {
			Provider     string         `json:"provider"`
			Model        string         `json:"model"`
			Type         string         `json:"type"`
			ArtifactKind string         `json:"artifactKind"`
			Input        map[string]any `json:"input"`
		} `json:"request"`
		Plan struct {
			Command       string `json:"command"`
			ArtifactKind  string `json:"artifactKind"`
			Submit        bool   `json:"submit"`
			Wait          bool   `json:"wait"`
			ResolveAssets bool   `json:"resolveAssets"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected dry-run JSON stdout, got %v: %s", err, stdout.String())
	}
	if !body.OK ||
		body.Status != "dry-run" ||
		!body.DryRun ||
		body.Request.Provider != "qiling" ||
		body.Request.Model != "gpt-image-2" ||
		body.Request.Type != "text-to-image" ||
		body.Request.ArtifactKind != "image" ||
		body.Request.Input["prompt"] != "a cinematic product photo" ||
		body.Plan.Command != "generateImage" ||
		body.Plan.ArtifactKind != "image" ||
		body.Plan.Submit ||
		body.Plan.Wait ||
		body.Plan.ResolveAssets {
		t.Fatalf("unexpected dry-run output %#v", body)
	}
}

func TestGenerateCommandRejectsUnsupportedCapabilityBeforeSubmit(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-test"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CLISchema {
			assertSchemaQuery(t, r, "qiling", "gpt-image-2", "text-to-image")
			return commandJSONResponse(http.StatusOK, remoteSchemaBody("qiling", "gpt-image-2", "text-to-image", "image")), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CapabilityModels("text-to-image") {
			return commandJSONResponse(http.StatusOK, `{"models":[]}`), nil
		}
		t.Fatalf("unsupported capability should not submit or poll, got %s %s", r.Method, r.URL.Path)
		return nil, nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateImage", "--provider", "qiling", "--model", "gpt-image-2", "--type", "text-to-image", "--input", "-"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		CommandStdin: strings.NewReader(`{"prompt":"a cinematic product photo"}`),
	})

	if exitCode == 0 {
		t.Fatalf("expected unsupported capability failure, stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON error, got %v: %s", err, stdout.String())
	}
	if body.Error.Code != errdefs.CodeCapabilityUnsupported {
		t.Fatalf("expected %s, got %q", errdefs.CodeCapabilityUnsupported, body.Error.Code)
	}
}

func TestGenerateCommandDownloadsArtifactToOutputPath(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-test"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	outputPath := filepath.Join(t.TempDir(), "result.png")
	httpClient := commandHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CLISchema {
			assertSchemaQuery(t, r, "qiling", "gpt-image-2", "text-to-image")
			return commandJSONResponse(http.StatusOK, remoteSchemaBody("qiling", "gpt-image-2", "text-to-image", "image")), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CapabilityModels("text-to-image") {
			return commandJSONResponse(http.StatusOK, remoteCapabilityModelsBody("qiling", "gpt-image-2", "text-to-image")), nil
		}
		if r.Method == http.MethodPost && r.URL.Path == serverapi.Generations {
			return commandJSONResponse(http.StatusOK, `{"jobId":"job_img_123","status":"queued"}`), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.TaskStatus("job_img_123") {
			return commandJSONResponse(http.StatusOK, `{"jobId":"job_img_123","status":"succeeded","artifacts":[{"url":"https://server.example/image.png","kind":"image","mime":"image/png"}]}`), nil
		}
		if r.Method == http.MethodGet && r.URL.String() == "https://server.example/image.png" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader("image-bytes"))}, nil
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		return nil, nil
	})
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateImage", "--provider", "qiling", "--model", "gpt-image-2", "--type", "text-to-image", "--input", "-", "--output", outputPath, "--timeout", "30", "--poll-interval", "1", "--poll-max", "1", "--poll-backoff", "1", "--no-jitter"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		HTTPClient:   httpClient,
		CommandStdin: strings.NewReader(`{"prompt":"lake"}`),
		TaskSleep:    func(delay time.Duration) {},
	})

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil || string(data) != "image-bytes" {
		t.Fatalf("expected downloaded output, data=%q err=%v", string(data), err)
	}
	var body struct {
		Artifacts []struct {
			URL  string `json:"url"`
			Path string `json:"path"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON stdout, got %v: %s", err, stdout.String())
	}
	if len(body.Artifacts) != 1 || body.Artifacts[0].Path != outputPath || body.Artifacts[0].URL != "https://server.example/image.png" {
		t.Fatalf("expected downloaded artifact path in stdout, got %#v", body.Artifacts)
	}
}

func TestGenerateCommandRejectsOutputWithNoWait(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	if err := config.Save(config.PathsFor(configDir), config.File{AuthToken: "sk-pipro-test"}); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}
	var stdout strings.Builder
	var stderr strings.Builder

	exitCode := ExecuteWithOptions([]string{"generateImage", "--provider", "qiling", "--model", "gpt-image-2", "--type", "text-to-image", "--input", "-", "--output", filepath.Join(t.TempDir(), "result.png"), "--no-wait"}, &stdout, &stderr, Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		CommandStdin: strings.NewReader(`{"prompt":"lake"}`),
	})

	if exitCode == 0 {
		t.Fatalf("expected non-zero exit")
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &body); err != nil {
		t.Fatalf("expected JSON error, got %v: %s", err, stdout.String())
	}
	if body.Error.Code != errdefs.CodeUsage {
		t.Fatalf("expected %s, got %q", errdefs.CodeUsage, body.Error.Code)
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
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CLISchema {
			assertSchemaQuery(t, r, "mock-provider", "mock-model", "text-to-image")
			return commandJSONResponse(http.StatusOK, remoteSchemaBody("mock-provider", "mock-model", "text-to-image", "image")), nil
		}
		if r.Method == http.MethodGet && r.URL.Path == serverapi.CapabilityModels("text-to-image") {
			return commandJSONResponse(http.StatusOK, remoteCapabilityModelsBody("mock-provider", "mock-model", "text-to-image")), nil
		}
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

func assertSchemaQuery(t *testing.T, r *http.Request, provider string, model string, schemaType string) {
	t.Helper()
	query := r.URL.Query()
	if query.Get("provider") != provider || query.Get("model") != model || query.Get("type") != schemaType {
		t.Fatalf("unexpected schema query: %s", r.URL.RawQuery)
	}
}

func remoteSchemaBody(provider string, model string, schemaType string, artifactKind string) string {
	return `{"ok":true,"schema":` + generateCommandSchemaJSON(provider, model, schemaType, artifactKind) + `}`
}

func remoteImageToImageSchemaBody(provider string, model string) string {
	return `{"ok":true,"schema":{
  "provider": "` + provider + `",
  "model": "` + model + `",
  "type": "image-to-image",
  "schemaVersion": "1.0",
  "artifactKind": "image",
  "input": {
    "required": ["prompt", "urls"],
    "properties": {
      "prompt": {"type": "string", "minLength": 1},
      "urls": {"type": "array"}
    }
  }
}}`
}

func remoteURIArraySchemaBody(provider string, model string, schemaType string, artifactKind string, field string) string {
	return `{"ok":true,"schema":{
  "provider": "` + provider + `",
  "model": "` + model + `",
  "type": "` + schemaType + `",
  "schemaVersion": "1.0",
  "artifactKind": "` + artifactKind + `",
  "input": {
    "required": ["prompt", "` + field + `"],
    "properties": {
      "prompt": {"type": "string", "minLength": 1},
      "` + field + `": {"type": "array", "items": {"type": "string", "format": "uri-or-base64"}}
    }
  }
}}`
}

func remoteCapabilityModelsBody(provider string, model string, schemaType string) string {
	return `{"models":[{"code":"` + model + `","name":"` + model + `","modality":"video","supportedEventTypes":["` + schemaType + `"],"providers":[{"providerCode":"` + provider + `","providerModelId":"` + model + `","healthStatus":"healthy"}]}]}`
}

func generateCommandSchemaJSON(provider string, model string, schemaType string, artifactKind string) string {
	return `{
  "provider": "` + provider + `",
  "model": "` + model + `",
  "type": "` + schemaType + `",
  "schemaVersion": "1.0",
  "artifactKind": "` + artifactKind + `",
  "input": {
    "required": ["prompt"],
    "properties": {
      "prompt": {"type": "string", "minLength": 1},
      "image": {"type": "array"}
    }
  }
}`
}

func writeTwoImageGenerateCommandSchema(t *testing.T, root string, provider string, model string) {
	t.Helper()
	path := filepath.Join(root, provider, model, "two-image-to-video.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("expected schema dir, got %v", err)
	}
	body := `{
  "provider": "` + provider + `",
  "model": "` + model + `",
  "type": "two-image-to-video",
  "schemaVersion": "1.0",
  "artifactKind": "video",
  "input": {
    "required": ["prompt", "image"],
    "properties": {
      "prompt": {"type": "string", "minLength": 1},
      "image": {"type": "array"}
    }
  }
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("expected schema write, got %v", err)
	}
}
