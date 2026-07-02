package generation

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/assets"
	"github.com/a754962942/pi-pro-cli/internal/client"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/input"
	"github.com/a754962942/pi-pro-cli/internal/schema"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
	"github.com/a754962942/pi-pro-cli/internal/task"
	"github.com/a754962942/pi-pro-cli/internal/validation"
)

type AssetResolver interface {
	Resolve(ctx context.Context, path string, options assets.ResolveOptions) (assets.Reference, error)
}

type CapabilityClient interface {
	CapabilityModels(ctx context.Context, eventType string) (client.CapabilityModelsResponse, error)
}

type Options struct {
	Registry      schema.Registry
	Capabilities  CapabilityClient
	AssetResolver AssetResolver
	AssetStore    *assets.Store
	ServerURL     string
	AuthToken     string
	HTTPClient    client.HTTPClient
	Stdin         inputReader
	Stderr        diagnosticWriter
	TaskSleep     func(delay time.Duration)
	TaskJitter    task.JitterFunc
}

type inputReader interface {
	Read(p []byte) (n int, err error)
}

type diagnosticWriter interface {
	Write(p []byte) (n int, err error)
}

type Request struct {
	Command       string
	ArtifactKind  string
	Provider      string
	Model         string
	Type          string
	InputPath     string
	CLIValues     map[string]any
	DryRun        bool
	Wait          bool
	WaitSpecified bool
	Timeout       time.Duration
	PollInterval  time.Duration
	PollMax       time.Duration
	PollBackoff   float64
	OutputPath    string
	OutputDir     string
}

type Result struct {
	OK        bool            `json:"ok"`
	Status    string          `json:"status"`
	JobID     string          `json:"jobId"`
	Provider  string          `json:"provider,omitempty"`
	Model     string          `json:"model,omitempty"`
	Type      string          `json:"type,omitempty"`
	DryRun    bool            `json:"dryRun,omitempty"`
	Request   any             `json:"request,omitempty"`
	Plan      *ExecutionPlan  `json:"plan,omitempty"`
	Artifacts []task.Artifact `json:"artifacts,omitempty"`
}

type ExecutionPlan struct {
	Command       string `json:"command"`
	ArtifactKind  string `json:"artifactKind"`
	Submit        bool   `json:"submit"`
	Wait          bool   `json:"wait"`
	ResolveAssets bool   `json:"resolveAssets"`
}

type Service struct {
	options Options
}

func NewService(options Options) Service {
	return Service{options: options}
}

func (s Service) Run(ctx context.Context, request Request) (Result, error) {
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	if err := validateOutputOptions(request); err != nil {
		return Result{}, err
	}
	if err := validateOutputTargets(request); err != nil {
		return Result{}, err
	}
	if s.options.Registry == nil {
		return Result{}, apperror.AppError{Code: errdefs.CodeSchemaNotFound, Message: "schema registry is not configured", Kind: apperror.KindValidation}
	}

	selected, err := s.options.Registry.Get(request.Provider, request.Model, request.Type)
	if err != nil {
		return Result{}, err
	}
	raw, err := input.Load(input.Options{InputPath: request.InputPath, Stdin: s.options.Stdin, CLIValues: request.CLIValues})
	if err != nil {
		return Result{}, err
	}
	normalized, err := validation.Normalize(request.ArtifactKind, selected, raw)
	if err != nil {
		return Result{}, err
	}
	if err := s.validateCapability(ctx, request, normalized); err != nil {
		return Result{}, err
	}
	if request.DryRun {
		return Result{
			OK:       true,
			Status:   "dry-run",
			Provider: request.Provider,
			Model:    request.Model,
			Type:     request.Type,
			DryRun:   true,
			Request:  normalized,
			Plan: &ExecutionPlan{
				Command:       request.Command,
				ArtifactKind:  request.ArtifactKind,
				Submit:        false,
				Wait:          false,
				ResolveAssets: false,
			},
		}, nil
	}
	if s.options.AuthToken == "" {
		return Result{}, apperror.AppError{
			Code:    errdefs.CodeAuthRequired,
			Message: "Authentication is required. Run pi-pro auth login first.",
			Kind:    apperror.KindAuth,
		}
	}
	if err := s.resolveFileFields(ctx, selected, normalized.Input); err != nil {
		return Result{}, err
	}

	var submitted task.Result
	api := client.New(client.Config{ServerURL: s.options.ServerURL, AuthToken: s.options.AuthToken, HTTPClient: s.options.HTTPClient})
	if err := api.DoJSON(ctx, http.MethodPost, serverapi.Generations, normalized, &submitted); err != nil {
		return Result{}, err
	}
	if submitted.JobID == "" {
		return Result{}, apperror.AppError{Code: errdefs.CodeServerResponseInvalid, Message: "generation response missing jobId", Kind: apperror.KindNetwork}
	}

	if task.IsTerminal(submitted.Status) {
		if submitted.Status != task.StatusSucceeded {
			return Result{}, terminalTaskError(submitted)
		}
		return s.resultFromTask(ctx, submitted, request, string(submitted.Status))
	}
	if !request.Wait {
		return Result{
			OK:       true,
			Status:   "submitted",
			JobID:    submitted.JobID,
			Provider: request.Provider,
			Model:    request.Model,
			Type:     request.Type,
		}, nil
	}

	waited, err := s.taskService().Wait(ctx, submitted.JobID, task.WaitOptions{
		Timeout:         request.Timeout,
		InitialInterval: request.PollInterval,
		MaxInterval:     request.PollMax,
		Backoff:         request.PollBackoff,
	})
	if err != nil {
		return Result{}, err
	}
	return s.resultFromTask(ctx, waited, request, string(waited.Status))
}

func (s Service) validateCapability(ctx context.Context, request Request, normalized validation.Request) error {
	if s.options.Capabilities == nil {
		return nil
	}
	models, err := s.options.Capabilities.CapabilityModels(ctx, normalized.Type)
	if err != nil {
		return err
	}
	for _, model := range models.Models {
		if model.Code != normalized.Model {
			continue
		}
		if !supportsEventType(model.SupportedEventTypes, normalized.Type) {
			continue
		}
		for _, provider := range model.Providers {
			if provider.ProviderCode == normalized.Provider {
				return nil
			}
		}
	}
	return apperror.AppError{
		Code:    errdefs.CodeCapabilityUnsupported,
		Message: "provider, model, and type are not advertised by server capabilities",
		Kind:    apperror.KindValidation,
		Details: map[string]any{
			"provider": normalized.Provider,
			"model":    normalized.Model,
			"type":     normalized.Type,
		},
	}
}

func supportsEventType(eventTypes []string, eventType string) bool {
	if len(eventTypes) == 0 {
		return true
	}
	for _, candidate := range eventTypes {
		if candidate == eventType {
			return true
		}
	}
	return false
}

func (s Service) resolveFileFields(ctx context.Context, selected schema.Schema, normalized map[string]any) error {
	if s.options.AssetResolver == nil {
		return nil
	}
	for field, mode := range fileResolveFields(selected.Input) {
		value, ok := normalized[field]
		if !ok {
			continue
		}
		path, ok := value.(string)
		if !ok {
			continue
		}
		ref, err := s.options.AssetResolver.Resolve(ctx, path, assets.ResolveOptions{Mode: mode})
		if err != nil {
			return err
		}
		normalized[field] = ref
	}
	for field := range uploadableStringArrayFields(selected.Input) {
		value, ok := normalized[field]
		if !ok {
			continue
		}
		resolved, changed, err := s.resolveLocalStrings(ctx, value)
		if err != nil {
			return err
		}
		if changed {
			normalized[field] = resolved
		}
	}
	return nil
}

func fileResolveFields(input map[string]any) map[string]assets.ResolveMode {
	fields := map[string]assets.ResolveMode{}
	properties, ok := input["properties"].(map[string]any)
	if !ok {
		return fields
	}
	for field, raw := range properties {
		property, ok := raw.(map[string]any)
		if !ok || property["type"] != "file" {
			continue
		}
		mode := assets.ResolveAssetDB
		if configured, ok := property["fileResolve"].(string); ok && configured != "" {
			mode = assets.ResolveMode(configured)
		}
		fields[field] = mode
	}
	return fields
}

func uploadableStringArrayFields(input map[string]any) map[string]bool {
	fields := map[string]bool{}
	properties, ok := input["properties"].(map[string]any)
	if !ok {
		return fields
	}
	for field, raw := range properties {
		property, ok := raw.(map[string]any)
		if !ok || property["type"] != "array" {
			continue
		}
		items, ok := property["items"].(map[string]any)
		if !ok || items["type"] != "string" {
			continue
		}
		if items["format"] == "uri-or-base64" {
			fields[field] = true
		}
	}
	return fields
}

func (s Service) resolveLocalStrings(ctx context.Context, value any) (any, bool, error) {
	items, ok := value.([]any)
	if !ok {
		return value, false, nil
	}
	resolved := make([]any, len(items))
	changed := false
	for i, item := range items {
		text, ok := item.(string)
		if !ok || !looksLocalFile(text) {
			resolved[i] = item
			continue
		}
		ref, err := s.options.AssetResolver.Resolve(ctx, text, assets.ResolveOptions{Mode: assets.ResolveAssetDBOrUpload})
		if err != nil {
			return nil, false, err
		}
		resolved[i] = ref
		changed = true
	}
	return resolved, changed, nil
}

func looksLocalFile(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" ||
		strings.HasPrefix(value, "http://") ||
		strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "data:") {
		return false
	}
	if _, err := os.Stat(value); err == nil {
		return true
	}
	if filepath.IsAbs(value) || strings.HasPrefix(value, ".") {
		return true
	}
	if strings.ContainsAny(value, `/\`) {
		return filepath.Ext(value) != ""
	}
	return filepath.Ext(value) != ""
}

func (s Service) taskService() task.Service {
	taskOptions := task.Options{
		ServerURL:  s.options.ServerURL,
		AuthToken:  s.options.AuthToken,
		HTTPClient: s.options.HTTPClient,
		Stderr:     s.options.Stderr,
		Jitter:     s.options.TaskJitter,
	}
	if s.options.TaskSleep != nil {
		taskOptions.Sleeper = func(ctx context.Context, delay time.Duration) error {
			s.options.TaskSleep(delay)
			return nil
		}
	}
	return task.NewService(taskOptions)
}

func (s Service) resultFromTask(ctx context.Context, result task.Result, request Request, status string) (Result, error) {
	artifacts := result.Artifacts
	if result.Status == task.StatusSucceeded && (request.OutputPath != "" || request.OutputDir != "") {
		downloader := ArtifactDownloader{
			ServerURL:  s.options.ServerURL,
			HTTPClient: s.options.HTTPClient,
			Store:      s.options.AssetStore,
		}
		downloaded, err := downloader.Download(ctx, artifacts, DownloadOptions{
			OutputPath:   request.OutputPath,
			OutputDir:    request.OutputDir,
			Provider:     request.Provider,
			Model:        request.Model,
			Type:         request.Type,
			JobID:        result.JobID,
			ArtifactKind: request.ArtifactKind,
		})
		if err != nil {
			return Result{}, err
		}
		artifacts = downloaded
	}
	return Result{
		OK:        true,
		Status:    status,
		JobID:     result.JobID,
		Provider:  request.Provider,
		Model:     request.Model,
		Type:      request.Type,
		Artifacts: artifacts,
	}, nil
}

func terminalTaskError(result task.Result) error {
	code := errdefs.CodeTaskFailed
	message := "Generation task failed."
	switch result.Status {
	case task.StatusCancelled:
		code = errdefs.CodeTaskCancelled
		message = "Generation task was cancelled."
	case task.StatusExpired:
		code = errdefs.CodeTaskExpired
		message = "Generation task expired."
	}
	details := map[string]any{"jobId": result.JobID, "status": string(result.Status)}
	if result.Error != nil {
		if result.Error.Code != "" {
			details["serverCode"] = result.Error.Code
		}
		if result.Error.Message != "" {
			details["serverMessage"] = result.Error.Message
		}
		for key, value := range result.Error.Details {
			if _, exists := details[key]; !exists {
				details[key] = value
			}
		}
	}
	return apperror.AppError{Code: code, Message: message, Kind: apperror.KindTask, Details: details}
}

func validateRequest(request Request) error {
	if request.Provider == "" || request.Model == "" || request.Type == "" {
		return apperror.Usage(errdefs.CodeUsage, "provider, model, and type are required")
	}
	if request.ArtifactKind == "" {
		return apperror.Usage(errdefs.CodeUsage, "artifact kind is required")
	}
	return nil
}

func validateOutputOptions(request Request) error {
	if request.OutputPath != "" && request.OutputDir != "" {
		return apperror.Usage(errdefs.CodeOutputPathAmbiguous, "output and output-dir cannot be used together")
	}
	if (request.OutputPath != "" || request.OutputDir != "") && !request.Wait {
		return apperror.Usage(errdefs.CodeUsage, "output download requires waiting for task completion")
	}
	return nil
}

func validateOutputTargets(request Request) error {
	if request.OutputPath == "" {
		return nil
	}
	if _, err := os.Stat(request.OutputPath); err == nil {
		return apperror.AppError{Code: errdefs.CodeOutputPathExists, Message: "output path already exists", Kind: apperror.KindIO, Details: map[string]any{"path": request.OutputPath}}
	} else if !os.IsNotExist(err) {
		return apperror.AppError{Code: errdefs.CodeArtifactDownloadFailed, Message: "failed to check output path", Kind: apperror.KindIO, Details: map[string]any{"path": request.OutputPath, "error": err.Error()}}
	}
	return nil
}
