package assets

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

type Asset struct {
	ID            string
	ServerAssetID string
	SourceURL     string
	MIME          string
	SizeBytes     int64
	SHA256        string
	Provider      string
	Model         string
	Type          string
	JobID         string
	ArtifactKind  string
}

type Location struct {
	AssetID  string
	FilePath string
}

type Record struct {
	ServerAssetID string
	SourceURL     string
	MIME          string
	Provider      string
	Model         string
	Type          string
	JobID         string
	ArtifactKind  string
	FilePath      string
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, assetDBOpenError(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, assetDBOpenError(err)
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) RecordDownloaded(record Record) (Asset, error) {
	return s.record(record)
}

func (s *Store) RecordUploaded(record Record) (Asset, error) {
	return s.record(record)
}

func (s *Store) FindByPath(path string) (Asset, bool, error) {
	normalized, err := normalizePath(path)
	if err != nil {
		return Asset{}, false, err
	}
	row := s.db.QueryRow(`
SELECT a.id, a.server_asset_id, a.source_url, a.mime, a.size_bytes, a.sha256, a.provider, a.model, a.type, a.job_id, a.artifact_kind
FROM assets a
JOIN asset_locations l ON l.asset_id = a.id
WHERE l.file_path = ?
LIMIT 1`, normalized)
	asset, err := scanAsset(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Asset{}, false, nil
	}
	if err != nil {
		return Asset{}, false, err
	}
	return asset, true, nil
}

func (s *Store) FindByContentPath(path string) (Asset, bool, error) {
	meta, err := fileMeta(path)
	if err != nil {
		return Asset{}, false, err
	}
	rows, err := s.db.Query(`
SELECT id, server_asset_id, source_url, mime, size_bytes, sha256, provider, model, type, job_id, artifact_kind
FROM assets
WHERE sha256 = ? AND size_bytes = ?`, meta.SHA256, meta.SizeBytes)
	if err != nil {
		return Asset{}, false, err
	}
	defer rows.Close()

	var matches []Asset
	for rows.Next() {
		asset, err := scanAssetRows(rows)
		if err != nil {
			return Asset{}, false, err
		}
		matches = append(matches, asset)
	}
	if err := rows.Err(); err != nil {
		return Asset{}, false, err
	}
	if len(matches) == 0 {
		return Asset{}, false, nil
	}
	if len(matches) > 1 {
		return Asset{}, false, apperror.AppError{
			Code:    errdefs.CodeAssetMatchAmbiguous,
			Message: "multiple assets matched local file content",
			Kind:    apperror.KindValidation,
			Details: map[string]any{"path": meta.Path},
		}
	}
	if err := s.addLocation(matches[0].ID, meta.Path); err != nil {
		return Asset{}, false, err
	}
	return matches[0], true, nil
}

func (s *Store) record(record Record) (Asset, error) {
	if record.SourceURL == "" {
		return Asset{}, assetDBWriteError(errors.New("sourceUrl is required"))
	}
	meta, err := fileMeta(record.FilePath)
	if err != nil {
		return Asset{}, err
	}
	id := record.ServerAssetID
	if id == "" {
		id = "sha256:" + meta.SHA256
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`
INSERT INTO assets (id, server_asset_id, source_url, mime, size_bytes, sha256, provider, model, type, job_id, artifact_kind, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  server_asset_id = excluded.server_asset_id,
  source_url = excluded.source_url,
  mime = excluded.mime,
  size_bytes = excluded.size_bytes,
  sha256 = excluded.sha256,
  provider = excluded.provider,
  model = excluded.model,
  type = excluded.type,
  job_id = excluded.job_id,
  artifact_kind = excluded.artifact_kind,
  updated_at = excluded.updated_at`,
		id, record.ServerAssetID, record.SourceURL, record.MIME, meta.SizeBytes, meta.SHA256, record.Provider, record.Model, record.Type, record.JobID, record.ArtifactKind, now, now)
	if err != nil {
		return Asset{}, assetDBWriteError(err)
	}
	if err := s.addLocation(id, meta.Path); err != nil {
		return Asset{}, err
	}
	asset, ok, err := s.FindByPath(meta.Path)
	if err != nil {
		return Asset{}, err
	}
	if !ok {
		return Asset{}, assetDBWriteError(errors.New("recorded asset could not be loaded"))
	}
	return asset, nil
}

func (s *Store) addLocation(assetID string, path string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
INSERT INTO asset_locations (asset_id, file_path, last_seen_at, file_exists, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?)
ON CONFLICT(asset_id, file_path) DO UPDATE SET
  last_seen_at = excluded.last_seen_at,
  file_exists = 1,
  updated_at = excluded.updated_at`, assetID, path, now, now, now)
	if err != nil {
		return assetDBWriteError(err)
	}
	return nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS assets (
  id TEXT PRIMARY KEY,
  server_asset_id TEXT,
  source_url TEXT NOT NULL,
  mime TEXT,
  size_bytes INTEGER NOT NULL,
  sha256 TEXT NOT NULL,
  provider TEXT,
  model TEXT,
  type TEXT,
  job_id TEXT,
  artifact_kind TEXT,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_assets_content ON assets(sha256, size_bytes);
CREATE TABLE IF NOT EXISTS asset_locations (
  asset_id TEXT NOT NULL,
  file_path TEXT NOT NULL,
  last_seen_at TEXT,
  file_exists INTEGER DEFAULT 1,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (asset_id, file_path)
);
CREATE INDEX IF NOT EXISTS idx_asset_locations_path ON asset_locations(file_path);
`)
	if err != nil {
		return assetDBOpenError(err)
	}
	if err := s.ensureColumns("assets", map[string]string{
		"provider":      "TEXT",
		"model":         "TEXT",
		"type":          "TEXT",
		"job_id":        "TEXT",
		"artifact_kind": "TEXT",
		"updated_at":    "TEXT",
	}); err != nil {
		return assetDBOpenError(err)
	}
	if err := s.ensureColumns("asset_locations", map[string]string{
		"last_seen_at": "TEXT",
		"file_exists":  "INTEGER DEFAULT 1",
		"updated_at":   "TEXT",
	}); err != nil {
		return assetDBOpenError(err)
	}
	return nil
}

func (s *Store) ensureColumns(table string, columns map[string]string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for name, definition := range columns {
		if existing[name] {
			continue
		}
		if _, err := s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + name + " " + definition); err != nil {
			return err
		}
	}
	return nil
}

type fileMetadata struct {
	Path      string
	SizeBytes int64
	SHA256    string
}

func fileMeta(path string) (fileMetadata, error) {
	normalized, err := normalizePath(path)
	if err != nil {
		return fileMetadata{}, err
	}
	info, err := os.Stat(normalized)
	if err != nil || info.IsDir() {
		if err == nil {
			err = fmt.Errorf("path is a directory")
		}
		return fileMetadata{}, apperror.AppError{
			Code:    errdefs.CodeInvalidFile,
			Message: "local file is invalid",
			Kind:    apperror.KindValidation,
			Details: map[string]any{"path": normalized, "error": err.Error()},
		}
	}
	data, err := os.ReadFile(normalized)
	if err != nil {
		return fileMetadata{}, apperror.AppError{
			Code:    errdefs.CodeInvalidFile,
			Message: "failed to read local file",
			Kind:    apperror.KindValidation,
			Details: map[string]any{"path": normalized, "error": err.Error()},
		}
	}
	sum := sha256.Sum256(data)
	return fileMetadata{Path: normalized, SizeBytes: info.Size(), SHA256: hex.EncodeToString(sum[:])}, nil
}

func normalizePath(path string) (string, error) {
	if path == "" {
		return "", apperror.AppError{Code: errdefs.CodeInvalidFile, Message: "local file path is required", Kind: apperror.KindValidation}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

type assetScanner interface {
	Scan(dest ...any) error
}

func scanAsset(row assetScanner) (Asset, error) {
	var asset Asset
	err := row.Scan(&asset.ID, &asset.ServerAssetID, &asset.SourceURL, &asset.MIME, &asset.SizeBytes, &asset.SHA256, &asset.Provider, &asset.Model, &asset.Type, &asset.JobID, &asset.ArtifactKind)
	return asset, err
}

func scanAssetRows(rows *sql.Rows) (Asset, error) {
	return scanAsset(rows)
}

func assetDBOpenError(err error) error {
	return apperror.AppError{Code: errdefs.CodeAssetDBOpenFailed, Message: "failed to open asset database", Kind: apperror.KindIO, Details: map[string]any{"error": err.Error()}}
}

func assetDBWriteError(err error) error {
	return apperror.AppError{Code: errdefs.CodeAssetDBWriteFailed, Message: "failed to write asset database", Kind: apperror.KindIO, Details: map[string]any{"error": err.Error()}}
}
