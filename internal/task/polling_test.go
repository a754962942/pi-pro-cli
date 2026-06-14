package task

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
)

func TestStatusFetchesTaskState(t *testing.T) {
	httpClient := taskHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != serverapi.TaskStatus("job_123") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return taskJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"running","progress":42}`), nil
	})

	result, err := NewService(Options{ServerURL: "https://api.example.test", HTTPClient: httpClient}).Status(context.Background(), "job_123")

	if err != nil {
		t.Fatalf("expected status, got %v", err)
	}
	if !result.OK || result.JobID != "job_123" || result.Status != StatusRunning || result.Progress == nil || *result.Progress != 42 {
		t.Fatalf("unexpected status result %#v", result)
	}
}

func TestWaitStopsWhenTaskReachesTerminalState(t *testing.T) {
	calls := 0
	httpClient := taskHTTPClient(func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			return taskJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"queued"}`), nil
		case 2:
			return taskJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"running"}`), nil
		case 3:
			return taskJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"succeeded","artifacts":[{"url":"https://server.example/video.mp4","mime":"video/mp4","kind":"video"}]}`), nil
		default:
			t.Fatalf("expected polling to stop after terminal state")
			return nil, nil
		}
	})
	var stderr bytes.Buffer
	var sleeps []time.Duration

	result, err := NewService(Options{
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
		Stderr:     &stderr,
		Sleeper: func(ctx context.Context, delay time.Duration) error {
			sleeps = append(sleeps, delay)
			return nil
		},
		Jitter: func(delay time.Duration) time.Duration {
			return delay
		},
	}).Wait(context.Background(), "job_123", WaitOptions{Timeout: time.Minute, InitialInterval: time.Second, MaxInterval: 5 * time.Second, Backoff: 2})

	if err != nil {
		t.Fatalf("expected wait success, got %v", err)
	}
	if result.Status != StatusSucceeded || len(result.Artifacts) != 1 {
		t.Fatalf("unexpected wait result %#v", result)
	}
	if calls != 3 || len(sleeps) != 2 {
		t.Fatalf("expected 3 calls and 2 sleeps, got calls=%d sleeps=%#v", calls, sleeps)
	}
	if stderr.String() == "" {
		t.Fatalf("expected progress diagnostics on stderr")
	}
}

func TestWaitTimeoutDoesNotCancelRemoteTask(t *testing.T) {
	var paths []string
	httpClient := taskHTTPClient(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodGet {
			t.Fatalf("expected only GET requests, got %s", r.Method)
		}
		return taskJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"running"}`), nil
	})

	_, err := NewService(Options{
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
		Sleeper: func(ctx context.Context, delay time.Duration) error {
			return nil
		},
		Jitter: func(delay time.Duration) time.Duration {
			return delay
		},
		Now: func() time.Time {
			return time.Unix(100, 0).UTC()
		},
	}).Wait(context.Background(), "job_123", WaitOptions{Timeout: 0, InitialInterval: time.Second, MaxInterval: time.Second, Backoff: 1})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeTaskTimeout {
		t.Fatalf("expected %s, got %q", errdefs.CodeTaskTimeout, appErr.Code)
	}
	for _, path := range paths {
		if path == serverapi.TaskCancel("job_123") {
			t.Fatalf("timeout must not cancel remote task")
		}
	}
}

func TestCancelCallsServerEndpoint(t *testing.T) {
	httpClient := taskHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != serverapi.TaskCancel("job_123") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return taskJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"cancelled"}`), nil
	})

	result, err := NewService(Options{ServerURL: "https://api.example.test", HTTPClient: httpClient}).Cancel(context.Background(), "job_123")

	if err != nil {
		t.Fatalf("expected cancel, got %v", err)
	}
	if !result.OK || result.JobID != "job_123" || result.Status != StatusCancelled {
		t.Fatalf("unexpected cancel result %#v", result)
	}
}

func TestWaitRetriesTransientNetworkErrorsWithinTimeout(t *testing.T) {
	calls := 0
	httpClient := taskHTTPClient(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("temporary network failure")
		}
		return taskJSONResponse(http.StatusOK, `{"jobId":"job_123","status":"succeeded"}`), nil
	})

	result, err := NewService(Options{
		ServerURL:  "https://api.example.test",
		HTTPClient: httpClient,
		Sleeper: func(ctx context.Context, delay time.Duration) error {
			return nil
		},
		Jitter: func(delay time.Duration) time.Duration {
			return delay
		},
	}).Wait(context.Background(), "job_123", WaitOptions{Timeout: time.Minute, InitialInterval: time.Second, MaxInterval: time.Second, Backoff: 1})

	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if result.Status != StatusSucceeded || calls != 2 {
		t.Fatalf("unexpected retry result %#v calls=%d", result, calls)
	}
}

func TestWaitDoesNotRetryNonTransientServerErrors(t *testing.T) {
	calls := 0
	httpClient := taskHTTPClient(func(r *http.Request) (*http.Response, error) {
		calls++
		return taskJSONResponse(http.StatusNotFound, `{"error":{"code":"TASK_NOT_FOUND","message":"task not found"}}`), nil
	})

	_, err := NewService(Options{ServerURL: "https://api.example.test", HTTPClient: httpClient}).Wait(context.Background(), "job_404", WaitOptions{Timeout: time.Minute})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeServerRequestFailed {
		t.Fatalf("expected %s, got %q", errdefs.CodeServerRequestFailed, appErr.Code)
	}
	if calls != 1 {
		t.Fatalf("expected no retry for 404, got %d calls", calls)
	}
}

type taskHTTPClient func(req *http.Request) (*http.Response, error)

func (f taskHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func taskJSONResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func TestTaskResultDoesNotExposeEmptyArtifactsAsNull(t *testing.T) {
	result := Result{OK: true, JobID: "job_123", Status: StatusRunning}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("expected marshal, got %v", err)
	}
	if bytes.Contains(data, []byte(`"artifacts":null`)) {
		t.Fatalf("expected empty artifacts to be omitted or array, got %s", data)
	}
}
