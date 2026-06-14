package assets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

func TestResolverUsesExistingAssetDBMapping(t *testing.T) {
	store := openTestStore(t)
	filePath := writeResolveFile(t, "known.png", []byte("known"))
	if _, err := store.RecordDownloaded(Record{ServerAssetID: "asset_123", SourceURL: "https://server.example/known.png", MIME: "image/png", FilePath: filePath}); err != nil {
		t.Fatalf("expected record, got %v", err)
	}

	ref, err := NewResolver(store, nil).Resolve(context.Background(), filePath, ResolveOptions{Mode: ResolveAssetDB})

	if err != nil {
		t.Fatalf("expected resolve, got %v", err)
	}
	if ref.Source != "asset-db" || ref.AssetID == "" || ref.Path != filePath || ref.URL != "https://server.example/known.png" || ref.MIME != "image/png" {
		t.Fatalf("unexpected reference %#v", ref)
	}
}

func TestResolverFailsUnknownFileWhenUploadIsNotAllowed(t *testing.T) {
	store := openTestStore(t)
	filePath := writeResolveFile(t, "unknown.png", []byte("unknown"))

	_, err := NewResolver(store, nil).Resolve(context.Background(), filePath, ResolveOptions{Mode: ResolveAssetDB})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeAssetURLNotFound {
		t.Fatalf("expected %s, got %q", errdefs.CodeAssetURLNotFound, appErr.Code)
	}
}

func TestResolverUploadsAndRecordsWhenUploadIsAllowed(t *testing.T) {
	store := openTestStore(t)
	filePath := writeResolveFile(t, "local.png", []byte("upload-me"))
	uploader := fakeUploader{
		response: UploadResult{
			ServerAssetID: "asset_uploaded",
			SourceURL:     "https://server.example/uploads/local.png",
			MIME:          "image/png",
		},
	}

	ref, err := NewResolver(store, &uploader).Resolve(context.Background(), filePath, ResolveOptions{Mode: ResolveUpload})

	if err != nil {
		t.Fatalf("expected upload resolve, got %v", err)
	}
	if ref.Source != "upload" || ref.AssetID == "" || ref.URL != "https://server.example/uploads/local.png" {
		t.Fatalf("unexpected uploaded reference %#v", ref)
	}
	found, ok, err := store.FindByPath(filePath)
	if err != nil {
		t.Fatalf("expected store lookup, got %v", err)
	}
	if !ok || found.ServerAssetID != "asset_uploaded" {
		t.Fatalf("expected uploaded asset recorded, got ok=%v asset=%#v", ok, found)
	}
}

func TestResolverAssetDBOrUploadReusesDBBeforeUpload(t *testing.T) {
	store := openTestStore(t)
	filePath := writeResolveFile(t, "known.png", []byte("known"))
	if _, err := store.RecordDownloaded(Record{ServerAssetID: "asset_123", SourceURL: "https://server.example/known.png", FilePath: filePath}); err != nil {
		t.Fatalf("expected record, got %v", err)
	}
	uploader := fakeUploader{response: UploadResult{ServerAssetID: "asset_uploaded", SourceURL: "https://server.example/uploads/local.png"}}

	ref, err := NewResolver(store, &uploader).Resolve(context.Background(), filePath, ResolveOptions{Mode: ResolveAssetDBOrUpload})

	if err != nil {
		t.Fatalf("expected resolve, got %v", err)
	}
	if ref.Source != "asset-db" || uploader.called {
		t.Fatalf("expected DB reuse without upload, ref=%#v called=%v", ref, uploader.called)
	}
}

func TestResolverFailsInvalidFile(t *testing.T) {
	store := openTestStore(t)
	missing := filepath.Join(t.TempDir(), "missing.png")

	_, err := NewResolver(store, nil).Resolve(context.Background(), missing, ResolveOptions{Mode: ResolveAssetDB})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeInvalidFile {
		t.Fatalf("expected %s, got %q", errdefs.CodeInvalidFile, appErr.Code)
	}
}

type fakeUploader struct {
	called   bool
	response UploadResult
	err      error
}

func (u *fakeUploader) Upload(ctx context.Context, path string, metadata UploadMetadata) (UploadResult, error) {
	u.called = true
	if u.err != nil {
		return UploadResult{}, u.err
	}
	return u.response, nil
}

func writeResolveFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("expected file write, got %v", err)
	}
	return path
}
