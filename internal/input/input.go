package input

import (
	"encoding/json"
	"io"
	"os"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
)

type Options struct {
	InputPath string
	Stdin     io.Reader
	CLIValues map[string]any
}

func Load(options Options) (map[string]any, error) {
	merged := copyMap(options.CLIValues)
	if options.InputPath == "" {
		return merged, nil
	}

	inputValues, err := readInputJSON(options)
	if err != nil {
		return nil, err
	}
	for key, value := range inputValues {
		merged[key] = value
	}
	return merged, nil
}

func readInputJSON(options Options) (map[string]any, error) {
	var data []byte
	var err error
	if options.InputPath == "-" {
		reader := options.Stdin
		if reader == nil {
			reader = os.Stdin
		}
		data, err = io.ReadAll(reader)
	} else {
		data, err = os.ReadFile(options.InputPath)
	}
	if err != nil {
		return nil, apperror.AppError{
			Code:    errdefs.CodeInvalidInputJSON,
			Message: "failed to read input JSON",
			Kind:    apperror.KindUsage,
			Details: map[string]any{"error": err.Error()},
		}
	}

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, apperror.AppError{
			Code:    errdefs.CodeInvalidInputJSON,
			Message: "input was not valid JSON",
			Kind:    apperror.KindUsage,
			Details: map[string]any{"error": err.Error()},
		}
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, apperror.AppError{
			Code:    errdefs.CodeInvalidInputJSON,
			Message: "input JSON must be an object",
			Kind:    apperror.KindUsage,
		}
	}
	return object, nil
}

func copyMap(values map[string]any) map[string]any {
	copied := make(map[string]any, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
