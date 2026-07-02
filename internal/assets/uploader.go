package assets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type HTTPUploader struct {
	ServerURL  string
	AuthToken  string
	HTTPClient HTTPClient
}

func (u HTTPUploader) Upload(ctx context.Context, path string, metadata UploadMetadata) (UploadResult, error) {
	body, contentType, err := multipartBody(path)
	if err != nil {
		return UploadResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.url(serverapi.AssetsUpload), body)
	if err != nil {
		return UploadResult{}, apperror.AppError{Code: errdefs.CodeUsage, Message: err.Error(), Kind: apperror.KindUsage}
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if u.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.AuthToken)
	}

	httpClient := u.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return UploadResult{}, uploadError(path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return UploadResult{}, uploadError(path, fmt.Errorf("status %d", resp.StatusCode))
	}

	var response struct {
		AssetID   string `json:"assetId"`
		URL       string `json:"url"`
		MIME      string `json:"mime"`
		SizeBytes int64  `json:"sizeBytes"`
		SHA256    string `json:"sha256"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return UploadResult{}, uploadError(path, err)
	}
	if response.URL == "" {
		return UploadResult{}, uploadError(path, fmt.Errorf("missing url"))
	}
	if response.SizeBytes == 0 {
		response.SizeBytes = metadata.SizeBytes
	}
	if response.SHA256 == "" {
		response.SHA256 = metadata.SHA256
	}
	return UploadResult{
		ServerAssetID: response.AssetID,
		SourceURL:     response.URL,
		MIME:          response.MIME,
		SizeBytes:     response.SizeBytes,
		SHA256:        response.SHA256,
	}, nil
}

func multipartBody(path string) (*bytes.Buffer, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", uploadError(path, err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return nil, "", uploadError(path, err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, "", uploadError(path, err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", uploadError(path, err)
	}
	return body, writer.FormDataContentType(), nil
}

func (u HTTPUploader) url(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(u.ServerURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func uploadError(path string, err error) error {
	return apperror.AppError{
		Code:    errdefs.CodeUploadFailed,
		Message: "asset upload failed",
		Kind:    apperror.KindNetwork,
		Details: map[string]any{"path": path, "error": err.Error()},
	}
}
