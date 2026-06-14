package validation

import (
	"math"
	"reflect"
	"strings"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/schema"
)

type Request struct {
	Provider     string         `json:"provider"`
	Model        string         `json:"model"`
	Type         string         `json:"type"`
	ArtifactKind string         `json:"artifactKind"`
	Input        map[string]any `json:"input"`
}

type FieldError struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type property struct {
	Type       string
	Default    any
	HasDefault bool
	Enum       []any
	Minimum    *float64
	Maximum    *float64
	MinLength  *int
	MaxLength  *int
}

type inputContract struct {
	UnknownPolicy string
	Required      map[string]bool
	Properties    map[string]property
}

func Normalize(commandArtifactKind string, selected schema.Schema, raw map[string]any) (Request, error) {
	if commandArtifactKind != "" && selected.ArtifactKind != commandArtifactKind {
		return Request{}, apperror.AppError{
			Code:    errdefs.CodeCommandTypeMismatch,
			Message: "schema artifact kind does not match command",
			Kind:    apperror.KindValidation,
			Details: map[string]any{
				"commandArtifactKind": commandArtifactKind,
				"schemaArtifactKind":  selected.ArtifactKind,
			},
		}
	}

	contract := parseInputContract(selected.Input)
	normalized := normalizeRaw(raw)
	var details []FieldError

	for field := range normalized {
		if _, ok := contract.Properties[field]; ok {
			continue
		}
		switch contract.UnknownPolicy {
		case "strip":
			delete(normalized, field)
		case "passthrough":
		default:
			details = append(details, fieldError(errdefs.CodeUnknownField, field, "Field is not allowed."))
		}
	}

	for field, prop := range contract.Properties {
		if _, ok := normalized[field]; ok {
			continue
		}
		if prop.HasDefault && !contract.Required[field] {
			normalized[field] = prop.Default
		}
	}

	for field := range contract.Required {
		if isMissing(normalized[field]) {
			details = append(details, fieldError(errdefs.CodeMissingRequiredField, field, "Field is required."))
		}
	}

	for field, value := range normalized {
		prop, ok := contract.Properties[field]
		if !ok {
			continue
		}
		details = append(details, validateValue(field, value, prop)...)
	}

	if len(details) > 0 {
		return Request{}, validationError(details)
	}
	return Request{
		Provider:     selected.Provider,
		Model:        selected.Model,
		Type:         selected.Type,
		ArtifactKind: selected.ArtifactKind,
		Input:        normalized,
	}, nil
}

func parseInputContract(input map[string]any) inputContract {
	contract := inputContract{
		UnknownPolicy: "reject",
		Required:      map[string]bool{},
		Properties:    map[string]property{},
	}
	if policy, ok := input["unknownPolicy"].(string); ok && policy != "" {
		contract.UnknownPolicy = policy
	}
	if required, ok := input["required"].([]any); ok {
		for _, item := range required {
			if field, ok := item.(string); ok && field != "" {
				contract.Required[field] = true
			}
		}
	}
	properties, ok := input["properties"].(map[string]any)
	if !ok {
		return contract
	}
	for field, rawProperty := range properties {
		rawMap, ok := rawProperty.(map[string]any)
		if !ok {
			continue
		}
		prop := property{}
		if value, ok := rawMap["type"].(string); ok {
			prop.Type = value
		}
		if value, ok := rawMap["default"]; ok {
			prop.Default = value
			prop.HasDefault = true
		}
		if values, ok := rawMap["enum"].([]any); ok {
			prop.Enum = values
		}
		prop.Minimum = optionalFloat(rawMap["minimum"])
		prop.Maximum = optionalFloat(rawMap["maximum"])
		prop.MinLength = optionalInt(rawMap["minLength"])
		prop.MaxLength = optionalInt(rawMap["maxLength"])
		if required, ok := rawMap["required"].(bool); ok && required {
			contract.Required[field] = true
		}
		contract.Properties[field] = prop
	}
	return contract
}

func normalizeRaw(raw map[string]any) map[string]any {
	normalized := make(map[string]any, len(raw))
	for key, value := range raw {
		if isMissing(value) {
			continue
		}
		normalized[key] = value
	}
	return normalized
}

func isMissing(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok && text == "" {
		return true
	}
	return false
}

func validateValue(field string, value any, prop property) []FieldError {
	var details []FieldError
	if prop.Type != "" && !matchesType(value, prop.Type) {
		return []FieldError{fieldError(errdefs.CodeInvalidFieldType, field, "Field type is invalid.")}
	}
	if len(prop.Enum) > 0 && !containsEnum(prop.Enum, value) {
		details = append(details, fieldError(errdefs.CodeInvalidEnumValue, field, "Field value is not allowed."))
	}
	if number, ok := numericValue(value); ok {
		if prop.Minimum != nil && number < *prop.Minimum {
			details = append(details, fieldError(errdefs.CodeInvalidRange, field, "Field value is below minimum."))
		}
		if prop.Maximum != nil && number > *prop.Maximum {
			details = append(details, fieldError(errdefs.CodeInvalidRange, field, "Field value is above maximum."))
		}
	}
	if text, ok := value.(string); ok {
		if prop.MinLength != nil && len(text) < *prop.MinLength {
			details = append(details, fieldError(errdefs.CodeInvalidRange, field, "Field length is below minimum."))
		}
		if prop.MaxLength != nil && len(text) > *prop.MaxLength {
			details = append(details, fieldError(errdefs.CodeInvalidRange, field, "Field length is above maximum."))
		}
	}
	return details
}

func matchesType(value any, expected string) bool {
	switch expected {
	case "string", "file":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := numericValue(value)
		return ok
	case "integer":
		number, ok := numericValue(value)
		return ok && math.Trunc(number) == number
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	default:
		return true
	}
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	default:
		return 0, false
	}
}

func containsEnum(values []any, value any) bool {
	for _, item := range values {
		if reflect.DeepEqual(item, value) {
			return true
		}
		if left, ok := numericValue(item); ok {
			if right, ok := numericValue(value); ok && left == right {
				return true
			}
		}
	}
	return false
}

func optionalFloat(value any) *float64 {
	number, ok := numericValue(value)
	if !ok {
		return nil
	}
	return &number
}

func optionalInt(value any) *int {
	number, ok := numericValue(value)
	if !ok {
		return nil
	}
	converted := int(number)
	return &converted
}

func validationError(details []FieldError) error {
	return apperror.AppError{
		Code:    errdefs.CodeValidationError,
		Message: "Input validation failed.",
		Kind:    apperror.KindValidation,
		Details: details,
	}
}

func fieldError(code string, field string, message string) FieldError {
	return FieldError{Code: code, Field: strings.TrimSpace(field), Message: message}
}
