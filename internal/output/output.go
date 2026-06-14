package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type errorEnvelope struct {
	OK    bool      `json:"ok"`
	Error errorBody `json:"error"`
}

func WriteJSON(w io.Writer, value any) int {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return 1
	}
	return 0
}

func WriteError(w io.Writer, err apperror.AppError) int {
	_ = WriteJSON(w, errorEnvelope{
		OK: false,
		Error: errorBody{
			Code:    err.Code,
			Message: redactString(err.Message),
			Details: redactValue(err.Details),
		},
	})
	return exitCode(err.Kind)
}

func WriteDiagnostic(w io.Writer, message string) {
	_, _ = fmt.Fprintln(w, redactString(message))
}

func exitCode(kind apperror.Kind) int {
	switch kind {
	case apperror.KindUsage, apperror.KindValidation:
		return 2
	case apperror.KindAuth:
		return 3
	case apperror.KindNetwork:
		return 4
	case apperror.KindTask:
		return 5
	case apperror.KindIO:
		return 6
	case apperror.KindLifecycle:
		return 7
	default:
		return 1
	}
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return redactString(typed)
	case []any:
		redacted := make([]any, 0, len(typed))
		for _, item := range typed {
			redacted = append(redacted, redactValue(item))
		}
		return redacted
	case map[string]string:
		redacted := make(map[string]string, len(typed))
		for key, item := range typed {
			if isSecretKey(key) {
				redacted[key] = "redacted"
				continue
			}
			redacted[key] = redactString(item)
		}
		return redacted
	case []map[string]string:
		redacted := make([]map[string]string, 0, len(typed))
		for _, item := range typed {
			next := make(map[string]string, len(item))
			for key, value := range item {
				if isSecretKey(key) {
					next[key] = "redacted"
					continue
				}
				next[key] = redactString(value)
			}
			redacted = append(redacted, next)
		}
		return redacted
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSecretKey(key) {
				redacted[key] = "redacted"
				continue
			}
			redacted[key] = redactValue(item)
		}
		return redacted
	default:
		return value
	}
}

func redactString(value string) string {
	redacted := value
	for _, marker := range []string{"authToken=", "password=", "Authorization: Bearer "} {
		offset := 0
		for {
			found := strings.Index(redacted[offset:], marker)
			if found == -1 {
				break
			}
			start := offset + found

			valueStart := start + len(marker)
			valueEnd := valueStart
			for valueEnd < len(redacted) && !isTokenBoundary(redacted[valueEnd]) {
				valueEnd++
			}
			redacted = redacted[:valueStart] + "redacted" + redacted[valueEnd:]
			offset = valueStart + len("redacted")
		}
	}
	return redacted
}

func isSecretKey(key string) bool {
	switch strings.ToLower(key) {
	case "authorization", "authtoken", "password", "requestbody":
		return true
	default:
		return false
	}
}

func isTokenBoundary(char byte) bool {
	switch char {
	case ' ', '\t', '\n', '\r', '"', '\'', ',', '}':
		return true
	default:
		return false
	}
}
