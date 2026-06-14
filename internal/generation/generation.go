package generation

import (
	"context"
	"net/http"
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

type Options struct {
	Registry      schema.Registry
	AssetResolver AssetResolver
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
	Wait          bool
	WaitSpecified bool
	Timeout       time.Duration
	PollInterval  time.Duration
	PollMax       time.Duration
	PollBackoff   float64
}

type Result struct {
	OK        bool            `json:"ok"`
	Status    string          `json:"status"`
	JobID     string          `json:"jobId"`
	Provider  string          `json:"provider,omitempty"`
	Model     string          `json:"model,omitempty"`
	Type      string          `json:"type,omitempty"`
	Artifacts []task.Artifact `json:"artifacts,omitempty"`
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
	if s.options.AuthToken == "" {
		return Result{}, apperror.AppError{
			Code:    errdefs.CodeAuthRequired,
			Message: "Authentication is required. Run pi-pro auth login first.",
			Kind:    apperror.KindAuth,
		}
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
		return resultFromTask(submitted, request, string(submitted.Status)), nil
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
	return resultFromTask(waited, request, string(waited.Status)), nil
}

func (s Service) resolveFileFields(ctx context.Context, selected schema.Schema, normalized map[string]any) error {
	if s.options.AssetResolver == nil {
		return nil
	}
	for field, mode := range fileResolveFields(selected) {
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
	return nil
}

func fileResolveFields(selected schema.Schema) map[string]assets.ResolveMode {
	fields := map[string]assets.ResolveMode{}
	properties, ok := selected.Input["properties"].(map[string]any)
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

func resultFromTask(result task.Result, request Request, status string) Result {
	return Result{
		OK:        true,
		Status:    status,
		JobID:     result.JobID,
		Provider:  request.Provider,
		Model:     request.Model,
		Type:      request.Type,
		Artifacts: result.Artifacts,
	}
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
