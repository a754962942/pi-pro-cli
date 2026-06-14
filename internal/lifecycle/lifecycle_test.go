package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestInitDownloadsManifestFilesAndInitializesState(t *testing.T) {
	schema := []byte(`{"provider":"seeddance","model":"v1","type":"image-to-video"}`)
	configDir := filepath.Join(t.TempDir(), "config")
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.0","updateAvailable":false,"updateRequired":false}`), nil
		case serverapi.CLIInitManifest:
			return jsonResponse(http.StatusOK, `{"version":"2026-06-14","files":[{"path":"schemas/seeddance/v1/image-to-video.json","url":"`+serverapi.CLIFilesPrefix+`schema.json","sha256":"`+testSHA256Hex(schema)+`","required":true}],"sqlite":{"assetDbSchemaVersion":1}}`), nil
		case serverapi.CLIFilesPrefix + "schema.json":
			return bytesResponse(http.StatusOK, schema), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
			return nil, nil
		}
	})

	result, err := Init(context.Background(), Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		LocalVersion: "0.1.0",
		HTTPClient:   httpClient,
	})

	if err != nil {
		t.Fatalf("expected init to succeed, got %v", err)
	}
	if !result.Initialized || !result.Changed {
		t.Fatalf("expected initialized changed result, got %#v", result)
	}
	if result.Files.Downloaded != 1 || result.Files.Skipped != 0 {
		t.Fatalf("expected one downloaded file, got %#v", result.Files)
	}

	schemaPath := filepath.Join(configDir, "schemas", "seeddance", "v1", "image-to-video.json")
	got, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("expected schema file, got %v", err)
	}
	if string(got) != string(schema) {
		t.Fatalf("expected schema %s, got %s", schema, got)
	}
	if _, err := os.Stat(filepath.Join(configDir, "init-state.json")); err != nil {
		t.Fatalf("expected init-state.json, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "version-state.json")); err != nil {
		t.Fatalf("expected version-state.json, got %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(configDir, "assets.sqlite"))
	if err != nil {
		t.Fatalf("expected sqlite open, got %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("expected sqlite ping, got %v", err)
	}
}

func TestInitRejectsInvalidManifestShapeBeforeDownloadingFiles(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	downloadRequested := false
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.0"}`), nil
		case serverapi.CLIInitManifest:
			return jsonResponse(http.StatusOK, `{"version":"2026-06-14","files":[{"path":"schemas/file.json","url":"","sha256":"abc","required":true}]}`), nil
		case serverapi.CLIFilesPrefix + "file.json":
			downloadRequested = true
			return bytesResponse(http.StatusOK, []byte("should not download")), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
			return nil, nil
		}
	})

	_, err := Init(context.Background(), Options{ConfigDir: configDir, ServerURL: "https://api.example.test", LocalVersion: "0.1.0", HTTPClient: httpClient})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeInitManifestInvalid {
		t.Fatalf("expected %s, got %q", errdefs.CodeInitManifestInvalid, appErr.Code)
	}
	if downloadRequested {
		t.Fatalf("expected invalid manifest to fail before file download")
	}
}

func TestInitIsIdempotentWhenFilesMatchChecksum(t *testing.T) {
	schema := []byte(`{"ok":true}`)
	configDir := filepath.Join(t.TempDir(), "config")
	requests := 0
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		requests++
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.0"}`), nil
		case serverapi.CLIInitManifest:
			return jsonResponse(http.StatusOK, `{"version":"2026-06-14","files":[{"path":"schemas/seeddance/v1/image-to-video.json","url":"`+serverapi.CLIFilesPrefix+`schema.json","sha256":"`+testSHA256Hex(schema)+`","required":true}]}`), nil
		case serverapi.CLIFilesPrefix + "schema.json":
			return bytesResponse(http.StatusOK, schema), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
			return nil, nil
		}
	})

	if _, err := Init(context.Background(), Options{ConfigDir: configDir, ServerURL: "https://api.example.test", LocalVersion: "0.1.0", HTTPClient: httpClient}); err != nil {
		t.Fatalf("expected first init to succeed, got %v", err)
	}
	requests = 0
	result, err := Init(context.Background(), Options{ConfigDir: configDir, ServerURL: "https://api.example.test", LocalVersion: "0.1.0", HTTPClient: httpClient})

	if err != nil {
		t.Fatalf("expected second init to succeed, got %v", err)
	}
	if result.Changed {
		t.Fatalf("expected second init unchanged, got %#v", result)
	}
	if result.Files.Downloaded != 0 || result.Files.Skipped != 1 {
		t.Fatalf("expected one skipped file, got %#v", result.Files)
	}
	if requests != 2 {
		t.Fatalf("expected only version and manifest requests on second init, got %d", requests)
	}
}

func TestInitRejectsPathTraversal(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.0"}`), nil
		case serverapi.CLIInitManifest:
			return jsonResponse(http.StatusOK, `{"version":"2026-06-14","files":[{"path":"../evil.json","url":"`+serverapi.CLIFilesPrefix+`evil.json","sha256":"abc","required":true}]}`), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
			return nil, nil
		}
	})

	_, err := Init(context.Background(), Options{ConfigDir: configDir, ServerURL: "https://api.example.test", LocalVersion: "0.1.0", HTTPClient: httpClient})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeInitPathInvalid {
		t.Fatalf("expected %s, got %q", errdefs.CodeInitPathInvalid, appErr.Code)
	}
}

func TestInitRejectsChecksumMismatch(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.0"}`), nil
		case serverapi.CLIInitManifest:
			return jsonResponse(http.StatusOK, `{"version":"2026-06-14","files":[{"path":"schemas/file.json","url":"`+serverapi.CLIFilesPrefix+`file.json","sha256":"bad","required":true}]}`), nil
		case serverapi.CLIFilesPrefix + "file.json":
			return bytesResponse(http.StatusOK, []byte("content")), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
			return nil, nil
		}
	})

	_, err := Init(context.Background(), Options{ConfigDir: configDir, ServerURL: "https://api.example.test", LocalVersion: "0.1.0", HTTPClient: httpClient})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeInitFileChecksumMismatch {
		t.Fatalf("expected %s, got %q", errdefs.CodeInitFileChecksumMismatch, appErr.Code)
	}
	if _, statErr := os.Stat(filepath.Join(configDir, "schemas", "file.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected invalid file not to be installed, stat err %v", statErr)
	}
}

func TestInitRequiresMatchingReleaseVersion(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != serverapi.CLIVersion {
			t.Fatalf("expected init to stop after version check, got %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.1"}`), nil
	})

	_, err := Init(context.Background(), Options{ConfigDir: configDir, ServerURL: "https://api.example.test", LocalVersion: "0.1.0", HTTPClient: httpClient})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeUpdateRequired {
		t.Fatalf("expected %s, got %q", errdefs.CodeUpdateRequired, appErr.Code)
	}
}

func TestUpdateReturnsUnchangedWhenAlreadyLatest(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != serverapi.CLIVersion {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.0"}`), nil
	})

	result, err := Update(context.Background(), Options{
		ConfigDir:    configDir,
		ServerURL:    "https://api.example.test",
		LocalVersion: "0.1.0",
		HTTPClient:   httpClient,
	})

	if err != nil {
		t.Fatalf("expected update to succeed, got %v", err)
	}
	if result.Changed {
		t.Fatalf("expected unchanged update, got %#v", result)
	}
	if result.LocalVersion != "0.1.0" || result.ReleaseVersion != "0.1.0" {
		t.Fatalf("expected version fields, got %#v", result)
	}
	if _, err := os.Stat(filepath.Join(configDir, "version-state.json")); err != nil {
		t.Fatalf("expected version-state.json to be written, got %v", err)
	}
}

func TestUpdateDownloadsManagedBinaryWhenReleaseDiffers(t *testing.T) {
	binary := []byte("#!/bin/sh\n")
	configDir := filepath.Join(t.TempDir(), "config")
	managedBinary := filepath.Join(configDir, "bin", "pi-pro")
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.1","binary":{"url":"`+serverapi.CLIReleaseBinary+`","sha256":"`+testSHA256Hex(binary)+`"}}`), nil
		case serverapi.CLIReleaseBinary:
			return bytesResponse(http.StatusOK, binary), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
			return nil, nil
		}
	})

	result, err := Update(context.Background(), Options{
		ConfigDir:      configDir,
		ServerURL:      "https://api.example.test",
		LocalVersion:   "0.1.0",
		HTTPClient:     httpClient,
		ExecutablePath: managedBinary,
	})

	if err != nil {
		t.Fatalf("expected update to succeed, got %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected update changed=true, got %#v", result)
	}
	installedPath := filepath.Join(configDir, "bin", "pi-pro")
	got, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("expected installed binary, got %v", err)
	}
	if string(got) != string(binary) {
		t.Fatalf("expected binary %q, got %q", binary, got)
	}
	info, err := os.Stat(installedPath)
	if err != nil {
		t.Fatalf("expected installed binary stat, got %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o755 {
		t.Fatalf("expected installed binary permission 0755, got %o", perm)
	}
}

func TestUpdateRejectsUnmanagedExecutableLocation(t *testing.T) {
	binary := []byte("#!/bin/sh\n")
	configDir := filepath.Join(t.TempDir(), "config")
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.1","binary":{"url":"`+serverapi.CLIReleaseBinary+`","sha256":"`+testSHA256Hex(binary)+`"}}`), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
			return nil, nil
		}
	})

	_, err := Update(context.Background(), Options{
		ConfigDir:      configDir,
		ServerURL:      "https://api.example.test",
		LocalVersion:   "0.1.0",
		HTTPClient:     httpClient,
		ExecutablePath: filepath.Join(t.TempDir(), "other", "pi-pro"),
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeUpdateUnsupportedInstallLocation {
		t.Fatalf("expected %s, got %q", errdefs.CodeUpdateUnsupportedInstallLocation, appErr.Code)
	}
}

func TestUpdateUsesUpdateChecksumErrorCode(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	managedBinary := filepath.Join(configDir, "bin", "pi-pro")
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.1","binary":{"url":"`+serverapi.CLIReleaseBinary+`","sha256":"bad"}}`), nil
		case serverapi.CLIReleaseBinary:
			return bytesResponse(http.StatusOK, []byte("new binary")), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
			return nil, nil
		}
	})

	_, err := Update(context.Background(), Options{
		ConfigDir:      configDir,
		ServerURL:      "https://api.example.test",
		LocalVersion:   "0.1.0",
		HTTPClient:     httpClient,
		ExecutablePath: managedBinary,
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeUpdateChecksumMismatch {
		t.Fatalf("expected %s, got %q", errdefs.CodeUpdateChecksumMismatch, appErr.Code)
	}
}

func TestUpdateOnWindowsRequiresHelperUpdater(t *testing.T) {
	binary := []byte("windows binary")
	configDir := filepath.Join(t.TempDir(), "config")
	managedBinary := filepath.Join(configDir, "bin", "pi-pro")
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.1","binary":{"url":"`+serverapi.CLIReleaseBinary+`","sha256":"`+testSHA256Hex(binary)+`"}}`), nil
		case serverapi.CLIReleaseBinary:
			return bytesResponse(http.StatusOK, binary), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
			return nil, nil
		}
	})

	_, err := Update(context.Background(), Options{
		ConfigDir:       configDir,
		ServerURL:       "https://api.example.test",
		LocalVersion:    "0.1.0",
		HTTPClient:      httpClient,
		ExecutablePath:  managedBinary,
		OperatingSystem: "windows",
	})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeUpdateHelperMissing {
		t.Fatalf("expected %s, got %q", errdefs.CodeUpdateHelperMissing, appErr.Code)
	}
	if _, err := os.Stat(managedBinary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected Windows path not to direct-replace managed binary, stat err %v", err)
	}
}

func TestUpdateOnWindowsStartsHelperAndWritesUpdateState(t *testing.T) {
	binary := []byte("windows binary")
	configDir := filepath.Join(t.TempDir(), "config")
	managedBinary := filepath.Join(configDir, "bin", "pi-pro")
	helperPath := filepath.Join(configDir, "bin", "pi-pro-updater.exe")
	if err := os.MkdirAll(filepath.Dir(helperPath), 0o700); err != nil {
		t.Fatalf("expected helper dir, got %v", err)
	}
	if err := os.WriteFile(helperPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("expected helper script, got %v", err)
	}
	httpClient := lifecycleHTTPClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case serverapi.CLIVersion:
			return jsonResponse(http.StatusOK, `{"localVersion":"0.1.0","releaseVersion":"0.1.1","binary":{"url":"`+serverapi.CLIReleaseBinary+`","sha256":"`+testSHA256Hex(binary)+`"}}`), nil
		case serverapi.CLIReleaseBinary:
			return bytesResponse(http.StatusOK, binary), nil
		default:
			t.Fatalf("unexpected request path %s", r.URL.Path)
			return nil, nil
		}
	})

	result, err := Update(context.Background(), Options{
		ConfigDir:       configDir,
		ServerURL:       "https://api.example.test",
		LocalVersion:    "0.1.0",
		HTTPClient:      httpClient,
		ExecutablePath:  managedBinary,
		OperatingSystem: "windows",
	})

	if err != nil {
		t.Fatalf("expected Windows update to start helper, got %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected changed update, got %#v", result)
	}
	if _, err := os.Stat(filepath.Join(configDir, "updates", "update-state.json")); err != nil {
		t.Fatalf("expected update-state.json, got %v", err)
	}
	if _, err := os.Stat(managedBinary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected Windows update not to direct-replace managed binary, stat err %v", err)
	}
}

func testSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type lifecycleHTTPClient func(req *http.Request) (*http.Response, error)

func (f lifecycleHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func bytesResponse(statusCode int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}
