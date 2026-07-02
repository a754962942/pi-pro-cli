package generation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/assets"
	"github.com/a754962942/pi-pro-cli/internal/client"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/task"
)

type ArtifactDownloader struct {
	ServerURL  string
	HTTPClient client.HTTPClient
	Store      *assets.Store
}

type DownloadOptions struct {
	OutputPath   string
	OutputDir    string
	Provider     string
	Model        string
	Type         string
	JobID        string
	ArtifactKind string
}

func (d ArtifactDownloader) Download(ctx context.Context, artifacts []task.Artifact, options DownloadOptions) ([]task.Artifact, error) {
	if len(artifacts) == 0 {
		return artifacts, nil
	}
	if options.OutputPath != "" && len(artifacts) != 1 {
		return nil, apperror.AppError{
			Code:    errdefs.CodeOutputPathAmbiguous,
			Message: "output can only be used when the task returns one artifact",
			Kind:    apperror.KindUsage,
			Details: map[string]any{"artifactCount": len(artifacts)},
		}
	}
	httpClient := d.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	downloaded := make([]task.Artifact, len(artifacts))
	copy(downloaded, artifacts)
	for i := range downloaded {
		if downloaded[i].URL == "" {
			return nil, artifactDownloadError(errors.New("artifact URL is required"), downloaded[i].URL)
		}
		target := options.OutputPath
		if target == "" {
			target = filepath.Join(options.OutputDir, artifactFileName(downloaded[i], options.JobID, i))
		}
		mimeType, err := d.downloadOne(ctx, httpClient, downloaded[i].URL, target)
		if err != nil {
			return nil, err
		}
		downloaded[i].Path = target
		if downloaded[i].MIME == "" {
			downloaded[i].MIME = mimeType
		}
		if d.Store != nil {
			if _, err := d.Store.RecordDownloaded(assets.Record{
				SourceURL:    downloaded[i].URL,
				MIME:         downloaded[i].MIME,
				Provider:     options.Provider,
				Model:        options.Model,
				Type:         options.Type,
				JobID:        options.JobID,
				ArtifactKind: options.ArtifactKind,
				FilePath:     target,
			}); err != nil {
				return nil, err
			}
		}
	}
	return downloaded, nil
}

func (d ArtifactDownloader) downloadOne(ctx context.Context, httpClient client.HTTPClient, rawURL string, target string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", artifactDownloadError(err, rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.artifactURL(rawURL), nil)
	if err != nil {
		return "", apperror.AppError{Code: errdefs.CodeUsage, Message: err.Error(), Kind: apperror.KindUsage}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", artifactDownloadError(err, rawURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", artifactDownloadError(fmt.Errorf("download returned status %d", resp.StatusCode), rawURL)
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", apperror.AppError{Code: errdefs.CodeOutputPathExists, Message: "output path already exists", Kind: apperror.KindIO, Details: map[string]any{"path": target}}
		}
		return "", artifactDownloadError(err, rawURL)
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", artifactDownloadError(err, rawURL)
	}
	return resp.Header.Get("Content-Type"), nil
}

func (d ArtifactDownloader) artifactURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	return strings.TrimRight(d.ServerURL, "/") + "/" + strings.TrimLeft(rawURL, "/")
}

func artifactFileName(artifact task.Artifact, jobID string, index int) string {
	ext := artifactExtension(artifact)
	return fmt.Sprintf("%s_%02d%s", safeFileSegment(jobID, "artifact"), index+1, ext)
}

func artifactExtension(artifact task.Artifact) string {
	if parsed, err := url.Parse(artifact.URL); err == nil {
		if ext := path.Ext(parsed.Path); ext != "" {
			return ext
		}
	}
	if artifact.MIME != "" {
		if exts, err := mime.ExtensionsByType(artifact.MIME); err == nil && len(exts) > 0 {
			return exts[0]
		}
	}
	return ""
}

func safeFileSegment(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return fallback
	}
	return builder.String()
}

func artifactDownloadError(err error, rawURL string) error {
	return apperror.AppError{
		Code:    errdefs.CodeArtifactDownloadFailed,
		Message: "failed to download artifact",
		Kind:    apperror.KindNetwork,
		Details: map[string]any{"url": rawURL, "error": err.Error()},
	}
}
