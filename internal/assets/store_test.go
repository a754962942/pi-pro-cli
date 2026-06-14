package assets

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	_ "modernc.org/sqlite"
)

func TestStoreRecordsAndFindsAssetByPath(t *testing.T) {
	store := openTestStore(t)
	filePath := writeAssetFile(t, "image-a.png", []byte("image-a"))

	record, err := store.RecordDownloaded(Record{
		ServerAssetID: "asset_123",
		SourceURL:     "https://server.example/image-a.png",
		MIME:          "image/png",
		Provider:      "seeddance",
		Model:         "v1",
		Type:          "image-to-video",
		ArtifactKind:  "image",
		FilePath:      filePath,
	})

	if err != nil {
		t.Fatalf("expected record, got %v", err)
	}
	found, ok, err := store.FindByPath(filePath)
	if err != nil {
		t.Fatalf("expected path lookup, got %v", err)
	}
	if !ok {
		t.Fatalf("expected asset by path")
	}
	if found.ID != record.ID || found.SourceURL != "https://server.example/image-a.png" || found.SHA256 == "" || found.SizeBytes != int64(len("image-a")) {
		t.Fatalf("unexpected asset %#v", found)
	}
}

func TestStoreResolvesMovedFileByContentAndRecordsNewLocation(t *testing.T) {
	store := openTestStore(t)
	originalPath := writeAssetFile(t, "original.png", []byte("same-content"))
	movedPath := writeAssetFile(t, "moved.png", []byte("same-content"))
	record, err := store.RecordDownloaded(Record{
		ServerAssetID: "asset_123",
		SourceURL:     "https://server.example/original.png",
		MIME:          "image/png",
		FilePath:      originalPath,
	})
	if err != nil {
		t.Fatalf("expected record, got %v", err)
	}

	found, ok, err := store.FindByContentPath(movedPath)

	if err != nil {
		t.Fatalf("expected content lookup, got %v", err)
	}
	if !ok || found.ID != record.ID {
		t.Fatalf("expected moved file to match original asset, got ok=%v asset=%#v", ok, found)
	}
	again, ok, err := store.FindByPath(movedPath)
	if err != nil {
		t.Fatalf("expected new path lookup, got %v", err)
	}
	if !ok || again.ID != record.ID {
		t.Fatalf("expected moved location recorded, got ok=%v asset=%#v", ok, again)
	}
}

func TestStoreReturnsAmbiguousWhenContentMatchesMultipleAssets(t *testing.T) {
	store := openTestStore(t)
	first := writeAssetFile(t, "first.png", []byte("same-content"))
	second := writeAssetFile(t, "second.png", []byte("same-content"))
	moved := writeAssetFile(t, "moved.png", []byte("same-content"))
	if _, err := store.RecordDownloaded(Record{ServerAssetID: "asset_1", SourceURL: "https://server.example/1.png", FilePath: first}); err != nil {
		t.Fatalf("expected first record, got %v", err)
	}
	if _, err := store.RecordDownloaded(Record{ServerAssetID: "asset_2", SourceURL: "https://server.example/2.png", FilePath: second}); err != nil {
		t.Fatalf("expected second record, got %v", err)
	}

	_, _, err := store.FindByContentPath(moved)

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeAssetMatchAmbiguous {
		t.Fatalf("expected %s, got %q", errdefs.CodeAssetMatchAmbiguous, appErr.Code)
	}
}

func TestOpenMigratesLegacyAssetTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "assets.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("expected sqlite open, got %v", err)
	}
	_, err = db.Exec(`
CREATE TABLE assets (
  id TEXT PRIMARY KEY,
  server_asset_id TEXT,
  source_url TEXT,
  mime TEXT,
  size_bytes INTEGER,
  sha256 TEXT,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE asset_locations (
  asset_id TEXT NOT NULL,
  file_path TEXT NOT NULL,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (asset_id, file_path)
);`)
	if err != nil {
		t.Fatalf("expected legacy schema, got %v", err)
	}
	_ = db.Close()
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("expected migrated store open, got %v", err)
	}
	defer store.Close()
	filePath := writeAssetFile(t, "legacy.png", []byte("legacy"))
	if _, err := store.RecordDownloaded(Record{ServerAssetID: "asset_legacy", SourceURL: "https://server.example/legacy.png", Provider: "seeddance", FilePath: filePath}); err != nil {
		t.Fatalf("expected record after migration, got %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "assets.sqlite"))
	if err != nil {
		t.Fatalf("expected store open, got %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func writeAssetFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("expected file write, got %v", err)
	}
	return path
}
