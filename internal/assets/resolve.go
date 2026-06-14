package assets

import (
	"context"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

type ResolveMode string

const (
	ResolveAssetDB         ResolveMode = "asset-db"
	ResolveUpload          ResolveMode = "upload"
	ResolveAssetDBOrUpload ResolveMode = "asset-db-or-upload"
)

type ResolveOptions struct {
	Mode ResolveMode
}

type Reference struct {
	Source  string `json:"source"`
	AssetID string `json:"assetId"`
	Path    string `json:"path"`
	URL     string `json:"url"`
	MIME    string `json:"mime,omitempty"`
}

type UploadMetadata struct {
	SHA256    string
	SizeBytes int64
}

type UploadResult struct {
	ServerAssetID string
	SourceURL     string
	MIME          string
	SizeBytes     int64
	SHA256        string
}

type Uploader interface {
	Upload(ctx context.Context, path string, metadata UploadMetadata) (UploadResult, error)
}

type Resolver struct {
	store    *Store
	uploader Uploader
}

func NewResolver(store *Store, uploader Uploader) Resolver {
	return Resolver{store: store, uploader: uploader}
}

func (r Resolver) Resolve(ctx context.Context, path string, options ResolveOptions) (Reference, error) {
	mode := options.Mode
	if mode == "" {
		mode = ResolveAssetDB
	}
	meta, err := fileMeta(path)
	if err != nil {
		return Reference{}, err
	}
	if mode == ResolveAssetDB || mode == ResolveAssetDBOrUpload {
		if ref, ok, err := r.resolveFromDB(meta.Path); err != nil {
			return Reference{}, err
		} else if ok {
			return ref, nil
		}
	}
	if mode == ResolveUpload || mode == ResolveAssetDBOrUpload {
		return r.upload(ctx, meta)
	}
	return Reference{}, apperror.AppError{
		Code:    errdefs.CodeAssetURLNotFound,
		Message: "local file has no known server URL mapping",
		Kind:    apperror.KindValidation,
		Details: map[string]any{"path": meta.Path},
	}
}

func (r Resolver) resolveFromDB(path string) (Reference, bool, error) {
	if asset, ok, err := r.store.FindByPath(path); err != nil {
		return Reference{}, false, err
	} else if ok {
		return referenceFromAsset("asset-db", path, asset), true, nil
	}
	asset, ok, err := r.store.FindByContentPath(path)
	if err != nil || !ok {
		return Reference{}, ok, err
	}
	return referenceFromAsset("asset-db", path, asset), true, nil
}

func (r Resolver) upload(ctx context.Context, meta fileMetadata) (Reference, error) {
	if r.uploader == nil {
		return Reference{}, apperror.AppError{
			Code:    errdefs.CodeUploadFailed,
			Message: "asset upload is not configured",
			Kind:    apperror.KindValidation,
			Details: map[string]any{"path": meta.Path},
		}
	}
	result, err := r.uploader.Upload(ctx, meta.Path, UploadMetadata{SHA256: meta.SHA256, SizeBytes: meta.SizeBytes})
	if err != nil {
		return Reference{}, apperror.AppError{
			Code:    errdefs.CodeUploadFailed,
			Message: "asset upload failed",
			Kind:    apperror.KindNetwork,
			Details: map[string]any{"path": meta.Path, "error": err.Error()},
		}
	}
	if result.SourceURL == "" {
		return Reference{}, apperror.AppError{
			Code:    errdefs.CodeUploadFailed,
			Message: "asset upload response missing durable URL",
			Kind:    apperror.KindNetwork,
			Details: map[string]any{"path": meta.Path},
		}
	}
	asset, err := r.store.RecordUploaded(Record{
		ServerAssetID: result.ServerAssetID,
		SourceURL:     result.SourceURL,
		MIME:          result.MIME,
		FilePath:      meta.Path,
	})
	if err != nil {
		return Reference{}, err
	}
	return referenceFromAsset("upload", meta.Path, asset), nil
}

func referenceFromAsset(source string, path string, asset Asset) Reference {
	return Reference{
		Source:  source,
		AssetID: asset.ID,
		Path:    path,
		URL:     asset.SourceURL,
		MIME:    asset.MIME,
	}
}
