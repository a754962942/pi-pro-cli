package task

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/client"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/output"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusExpired   Status = "expired"
)

func IsTerminal(status Status) bool {
	switch status {
	case StatusSucceeded, StatusFailed, StatusCancelled, StatusExpired:
		return true
	default:
		return false
	}
}

type HTTPClient = client.HTTPClient

type Artifact struct {
	URL  string `json:"url"`
	Path string `json:"path,omitempty"`
	MIME string `json:"mime,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type Result struct {
	OK        bool       `json:"ok"`
	JobID     string     `json:"jobId"`
	Status    Status     `json:"status"`
	Progress  *int       `json:"progress,omitempty"`
	Provider  string     `json:"provider,omitempty"`
	Model     string     `json:"model,omitempty"`
	Type      string     `json:"type,omitempty"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
	Error     *TaskError `json:"error,omitempty"`
}

type TaskError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type Options struct {
	ServerURL  string
	AuthToken  string
	HTTPClient HTTPClient
	Stderr     io.Writer
	Sleeper    Sleeper
	Jitter     JitterFunc
	Now        func() time.Time
}

type WaitOptions struct {
	Timeout         time.Duration
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Backoff         float64
}

type Sleeper func(ctx context.Context, delay time.Duration) error

type JitterFunc func(delay time.Duration) time.Duration

type Service struct {
	api     *client.Client
	stderr  io.Writer
	sleep   Sleeper
	jitter  JitterFunc
	now     func() time.Time
	randSrc *rand.Rand
}

func NewService(options Options) Service {
	if options.Sleeper == nil {
		options.Sleeper = sleepContext
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	service := Service{
		api:     client.New(client.Config{ServerURL: options.ServerURL, AuthToken: options.AuthToken, HTTPClient: options.HTTPClient}),
		stderr:  options.Stderr,
		sleep:   options.Sleeper,
		jitter:  options.Jitter,
		now:     options.Now,
		randSrc: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if service.jitter == nil {
		service.jitter = service.defaultJitter
	}
	return service
}

func (s Service) Status(ctx context.Context, jobID string) (Result, error) {
	if err := validateJobID(jobID); err != nil {
		return Result{}, err
	}
	var result Result
	if err := s.api.DoJSON(ctx, http.MethodGet, serverapi.TaskStatus(jobID), nil, &result); err != nil {
		return Result{}, err
	}
	return normalizeResult(result, jobID)
}

func (s Service) Cancel(ctx context.Context, jobID string) (Result, error) {
	if err := validateJobID(jobID); err != nil {
		return Result{}, err
	}
	var result Result
	if err := s.api.DoJSON(ctx, http.MethodPost, serverapi.TaskCancel(jobID), map[string]any{}, &result); err != nil {
		return Result{}, err
	}
	return normalizeResult(result, jobID)
}

func (s Service) Wait(ctx context.Context, jobID string, options WaitOptions) (Result, error) {
	options = normalizeWaitOptions(options)
	deadline := s.now().Add(options.Timeout)
	delay := options.InitialInterval
	var last Result

	for {
		result, err := s.Status(ctx, jobID)
		if err == nil {
			last = result
			if IsTerminal(result.Status) {
				return terminalResult(result)
			}
		} else if !isRetryablePollingError(err) {
			return Result{}, err
		}

		if !s.now().Before(deadline) {
			return Result{}, taskTimeout(jobID, last.Status)
		}

		actualDelay := s.jitter(delay)
		if s.stderr != nil {
			status := string(last.Status)
			if status == "" {
				status = "unknown"
			}
			output.WriteDiagnostic(s.stderr, fmt.Sprintf("task %s %s; polling again in %.1fs", jobID, status, actualDelay.Seconds()))
		}
		if err := s.sleep(ctx, actualDelay); err != nil {
			return Result{}, err
		}
		delay = nextDelay(delay, options.MaxInterval, options.Backoff)
	}
}

func normalizeResult(result Result, fallbackJobID string) (Result, error) {
	if result.JobID == "" {
		result.JobID = fallbackJobID
	}
	if result.Status == "" {
		return Result{}, apperror.AppError{
			Code:    errdefs.CodeTaskStatusInvalid,
			Message: "task response missing status",
			Kind:    apperror.KindNetwork,
			Details: map[string]any{"jobId": result.JobID},
		}
	}
	if !isKnownStatus(result.Status) {
		return Result{}, apperror.AppError{
			Code:    errdefs.CodeTaskStatusInvalid,
			Message: "task response contained an unknown status",
			Kind:    apperror.KindNetwork,
			Details: map[string]any{"jobId": result.JobID, "status": string(result.Status)},
		}
	}
	result.OK = true
	return result, nil
}

func terminalResult(result Result) (Result, error) {
	switch result.Status {
	case StatusSucceeded:
		result.OK = true
		return result, nil
	case StatusFailed:
		return Result{}, terminalTaskError(result, errdefs.CodeTaskFailed, "Generation task failed.")
	case StatusCancelled:
		return Result{}, terminalTaskError(result, errdefs.CodeTaskCancelled, "Generation task was cancelled.")
	case StatusExpired:
		return Result{}, terminalTaskError(result, errdefs.CodeTaskExpired, "Generation task expired.")
	default:
		return result, nil
	}
}

func terminalTaskError(result Result, code string, message string) error {
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

func taskTimeout(jobID string, lastStatus Status) error {
	details := map[string]any{"jobId": jobID}
	if lastStatus != "" {
		details["lastStatus"] = string(lastStatus)
	}
	return apperror.AppError{
		Code:    errdefs.CodeTaskTimeout,
		Message: "Task did not reach a terminal state before timeout.",
		Kind:    apperror.KindTask,
		Details: details,
	}
}

func normalizeWaitOptions(options WaitOptions) WaitOptions {
	if options.Timeout < 0 {
		options.Timeout = 0
	}
	if options.Timeout == 0 {
		return options
	}
	if options.InitialInterval <= 0 {
		options.InitialInterval = 2 * time.Second
	}
	if options.MaxInterval <= 0 {
		options.MaxInterval = 30 * time.Second
	}
	if options.Backoff <= 0 {
		options.Backoff = 1.5
	}
	if options.MaxInterval < options.InitialInterval {
		options.MaxInterval = options.InitialInterval
	}
	return options
}

func nextDelay(current time.Duration, maxDelay time.Duration, backoff float64) time.Duration {
	next := time.Duration(float64(current) * backoff)
	if next <= 0 {
		next = current
	}
	if next > maxDelay {
		return maxDelay
	}
	return next
}

func (s Service) defaultJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return delay
	}
	factor := 0.8 + s.randSrc.Float64()*0.4
	return time.Duration(float64(delay) * factor)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryablePollingError(err error) bool {
	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		return false
	}
	if appErr.Code == errdefs.CodeNetworkRequestFailed {
		return true
	}
	if appErr.Code != errdefs.CodeServerRequestFailed {
		return false
	}
	statusCode, ok := serverStatusCode(appErr.Details)
	return ok && (statusCode == http.StatusTooManyRequests || statusCode >= 500)
}

func serverStatusCode(details any) (int, bool) {
	items, ok := details.(map[string]any)
	if !ok {
		return 0, false
	}
	switch value := items["statusCode"].(type) {
	case int:
		return value, true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func isKnownStatus(status Status) bool {
	switch status {
	case StatusQueued, StatusRunning, StatusSucceeded, StatusFailed, StatusCancelled, StatusExpired:
		return true
	default:
		return false
	}
}

func validateJobID(jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return apperror.Usage(errdefs.CodeUsage, "jobId is required")
	}
	return nil
}
