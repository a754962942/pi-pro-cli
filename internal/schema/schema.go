package schema

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

type Schema struct {
	Provider      string         `json:"provider"`
	Model         string         `json:"model"`
	Type          string         `json:"type"`
	SchemaVersion string         `json:"schemaVersion"`
	ArtifactKind  string         `json:"artifactKind"`
	DisplayName   string         `json:"displayName,omitempty"`
	Description   string         `json:"description,omitempty"`
	Input         map[string]any `json:"input"`
}

type Summary struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Type         string `json:"type"`
	ArtifactKind string `json:"artifactKind"`
	DisplayName  string `json:"displayName,omitempty"`
}

type Registry interface {
	Get(provider string, model string, schemaType string) (Schema, error)
	List() ([]Summary, error)
}

type LocalRegistry struct {
	root string
}

type RemoteClient interface {
	Schema(ctx context.Context, provider string, model string, schemaType string) (json.RawMessage, error)
}

type RemoteRegistry struct {
	ctx      context.Context
	client   RemoteClient
	fallback Registry
}

func NewLocalRegistry(root string) LocalRegistry {
	return LocalRegistry{root: root}
}

func NewRemoteRegistry(ctx context.Context, client RemoteClient, fallback Registry) RemoteRegistry {
	if ctx == nil {
		ctx = context.Background()
	}
	return RemoteRegistry{ctx: ctx, client: client, fallback: fallback}
}

func (r RemoteRegistry) Get(provider string, model string, schemaType string) (Schema, error) {
	if r.client != nil {
		body, err := r.client.Schema(r.ctx, provider, model, schemaType)
		if err == nil {
			selected, err := decodeSchema(body)
			if err != nil {
				return Schema{}, err
			}
			if err := validateMetadata(selected, provider, model, schemaType); err != nil {
				return Schema{}, err
			}
			if err := validateShape(selected); err != nil {
				return Schema{}, err
			}
			return selected, nil
		}
	}
	if r.fallback != nil {
		return r.fallback.Get(provider, model, schemaType)
	}
	return Schema{}, apperror.AppError{
		Code:    errdefs.CodeSchemaNotFound,
		Message: "schema was not found",
		Kind:    apperror.KindValidation,
		Details: map[string]any{"provider": provider, "model": model, "type": schemaType},
	}
}

func (r RemoteRegistry) List() ([]Summary, error) {
	if r.fallback == nil {
		return []Summary{}, nil
	}
	return r.fallback.List()
}

func (r LocalRegistry) Get(provider string, model string, schemaType string) (Schema, error) {
	path, err := r.pathFor(provider, model, schemaType)
	if err != nil {
		return Schema{}, err
	}
	schema, err := readSchema(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Schema{}, apperror.AppError{
				Code:    errdefs.CodeSchemaNotFound,
				Message: "schema was not found",
				Kind:    apperror.KindValidation,
				Details: map[string]any{"provider": provider, "model": model, "type": schemaType},
			}
		}
		return Schema{}, err
	}
	if err := validateMetadata(schema, provider, model, schemaType); err != nil {
		return Schema{}, err
	}
	if err := validateShape(schema); err != nil {
		return Schema{}, err
	}
	return schema, nil
}

func (r LocalRegistry) List() ([]Summary, error) {
	var summaries []Summary
	err := filepath.WalkDir(r.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		provider, model, schemaType, ok := identityFromPath(r.root, path)
		if !ok {
			return nil
		}
		schema, err := r.Get(provider, model, schemaType)
		if err != nil {
			return err
		}
		summaries = append(summaries, Summary{
			Provider:     schema.Provider,
			Model:        schema.Model,
			Type:         schema.Type,
			ArtifactKind: schema.ArtifactKind,
			DisplayName:  schema.DisplayName,
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Summary{}, nil
		}
		return nil, err
	}
	sort.Slice(summaries, func(i int, j int) bool {
		left := summaries[i].Provider + "\x00" + summaries[i].Model + "\x00" + summaries[i].Type
		right := summaries[j].Provider + "\x00" + summaries[j].Model + "\x00" + summaries[j].Type
		return left < right
	})
	return summaries, nil
}

func (r LocalRegistry) pathFor(provider string, model string, schemaType string) (string, error) {
	for key, value := range map[string]string{"provider": provider, "model": model, "type": schemaType} {
		if value == "" || filepath.IsAbs(value) || strings.Contains(value, "/") || strings.Contains(value, "\\") || value == "." || value == ".." {
			return "", apperror.AppError{
				Code:    errdefs.CodeUsage,
				Message: "schema identity contains invalid path segment",
				Kind:    apperror.KindUsage,
				Details: map[string]any{"field": key},
			}
		}
	}
	return filepath.Join(r.root, provider, model, schemaType+".json"), nil
}

func readSchema(path string) (Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Schema{}, err
	}
	return decodeSchema(data)
}

func decodeSchema(data []byte) (Schema, error) {
	var schema Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return Schema{}, apperror.AppError{
			Code:    errdefs.CodeSchemaInvalid,
			Message: "schema file is not valid JSON",
			Kind:    apperror.KindValidation,
			Details: map[string]any{"error": err.Error()},
		}
	}
	return schema, nil
}

func validateMetadata(schema Schema, provider string, model string, schemaType string) error {
	if schema.Provider == provider && schema.Model == model && schema.Type == schemaType {
		return nil
	}
	return apperror.AppError{
		Code:    errdefs.CodeSchemaMetadataMismatch,
		Message: "schema metadata does not match its path",
		Kind:    apperror.KindValidation,
		Details: map[string]any{
			"expectedProvider": provider,
			"expectedModel":    model,
			"expectedType":     schemaType,
			"actualProvider":   schema.Provider,
			"actualModel":      schema.Model,
			"actualType":       schema.Type,
		},
	}
}

func validateShape(schema Schema) error {
	if schema.Provider == "" || schema.Model == "" || schema.Type == "" || schema.SchemaVersion == "" || schema.ArtifactKind == "" || schema.Input == nil {
		return apperror.AppError{
			Code:    errdefs.CodeSchemaInvalid,
			Message: "schema is missing required fields",
			Kind:    apperror.KindValidation,
		}
	}
	return nil
}

func identityFromPath(root string, path string) (string, string, string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", "", "", false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 3 {
		return "", "", "", false
	}
	schemaType := strings.TrimSuffix(parts[2], ".json")
	if schemaType == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], schemaType, true
}
