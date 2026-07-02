package schema

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

func TestLocalRegistryGetReturnsValidatedSchema(t *testing.T) {
	root := filepath.Join(t.TempDir(), "schemas")
	writeSchema(t, root, "seeddance", "v1", "image-to-video", `{
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video",
  "schemaVersion": "1.0",
  "artifactKind": "video",
  "displayName": "SeedDance Image to Video",
  "input": {"properties": {"prompt": {"type": "string"}}}
}`)

	registry := NewLocalRegistry(root)
	got, err := registry.Get("seeddance", "v1", "image-to-video")

	if err != nil {
		t.Fatalf("expected schema, got %v", err)
	}
	if got.Provider != "seeddance" || got.Model != "v1" || got.Type != "image-to-video" || got.ArtifactKind != "video" {
		t.Fatalf("unexpected schema %#v", got)
	}
}

func TestLocalRegistryListReturnsSortedSummaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "schemas")
	writeSchema(t, root, "seeddance", "v1", "text-to-video", schemaJSON("seeddance", "v1", "text-to-video", "video"))
	writeSchema(t, root, "happy-horse", "v2", "image-to-video", schemaJSON("happy-horse", "v2", "image-to-video", "video"))

	registry := NewLocalRegistry(root)
	got, err := registry.List()

	if err != nil {
		t.Fatalf("expected list, got %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 schemas, got %#v", got)
	}
	if got[0].Provider != "happy-horse" || got[1].Provider != "seeddance" {
		t.Fatalf("expected sorted schemas, got %#v", got)
	}
}

func TestLocalRegistryRejectsMetadataMismatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "schemas")
	writeSchema(t, root, "seeddance", "v1", "image-to-video", schemaJSON("other", "v1", "image-to-video", "video"))

	_, err := NewLocalRegistry(root).Get("seeddance", "v1", "image-to-video")

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeSchemaMetadataMismatch {
		t.Fatalf("expected %s, got %q", errdefs.CodeSchemaMetadataMismatch, appErr.Code)
	}
}

func TestLocalRegistryRejectsInvalidSchema(t *testing.T) {
	root := filepath.Join(t.TempDir(), "schemas")
	writeSchema(t, root, "seeddance", "v1", "image-to-video", `{"provider":"seeddance"`)

	_, err := NewLocalRegistry(root).Get("seeddance", "v1", "image-to-video")

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeSchemaInvalid {
		t.Fatalf("expected %s, got %q", errdefs.CodeSchemaInvalid, appErr.Code)
	}
}

func TestLocalRegistryReturnsNotFoundForUnknownSchema(t *testing.T) {
	_, err := NewLocalRegistry(filepath.Join(t.TempDir(), "schemas")).Get("seeddance", "v1", "image-to-video")

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeSchemaNotFound {
		t.Fatalf("expected %s, got %q", errdefs.CodeSchemaNotFound, appErr.Code)
	}
}

func TestRemoteRegistryGetsSchemaFromAPI(t *testing.T) {
	registry := NewRemoteRegistry(context.Background(), fakeRemoteClient{
		body: json.RawMessage(`{"provider":"vidu","model":"vidu/viduq3-turbo_img2video","type":"image-to-video","schemaVersion":"1.0","artifactKind":"video","input":{}}`),
	}, nil)

	got, err := registry.Get("vidu", "vidu/viduq3-turbo_img2video", "image-to-video")

	if err != nil {
		t.Fatalf("expected remote schema, got %v", err)
	}
	if got.Model != "vidu/viduq3-turbo_img2video" {
		t.Fatalf("unexpected schema: %#v", got)
	}
}

func TestRemoteRegistryFallsBackToLocalSchema(t *testing.T) {
	root := filepath.Join(t.TempDir(), "schemas")
	writeSchema(t, root, "seeddance", "v1", "image-to-video", schemaJSON("seeddance", "v1", "image-to-video", "video"))
	registry := NewRemoteRegistry(context.Background(), fakeRemoteClient{err: errors.New("offline")}, NewLocalRegistry(root))

	got, err := registry.Get("seeddance", "v1", "image-to-video")

	if err != nil {
		t.Fatalf("expected fallback schema, got %v", err)
	}
	if got.Provider != "seeddance" || got.Model != "v1" {
		t.Fatalf("unexpected fallback schema: %#v", got)
	}
}

type fakeRemoteClient struct {
	body json.RawMessage
	err  error
}

func (c fakeRemoteClient) Schema(ctx context.Context, provider string, model string, schemaType string) (json.RawMessage, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.body, nil
}

func writeSchema(t *testing.T, root string, provider string, model string, schemaType string, body string) {
	t.Helper()
	path := filepath.Join(root, provider, model, schemaType+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("expected schema dir, got %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("expected schema write, got %v", err)
	}
}

func schemaJSON(provider string, model string, schemaType string, artifactKind string) string {
	return `{
  "provider": "` + provider + `",
  "model": "` + model + `",
  "type": "` + schemaType + `",
  "schemaVersion": "1.0",
  "artifactKind": "` + artifactKind + `",
  "displayName": "` + provider + ` ` + schemaType + `",
  "input": {"properties": {"prompt": {"type": "string"}}}
}`
}
