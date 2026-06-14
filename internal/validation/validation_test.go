package validation

import (
	"errors"
	"testing"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/schema"
)

func TestNormalizeAppliesDefaultsAndPreservesFalseAndZero(t *testing.T) {
	request, err := Normalize("video", testSchema(), map[string]any{
		"prompt":      "lake",
		"image":       "lake.png",
		"cameraFixed": false,
		"seed":        0,
	})

	if err != nil {
		t.Fatalf("expected normalized request, got %v", err)
	}
	if request.Provider != "seeddance" || request.ArtifactKind != "video" {
		t.Fatalf("unexpected request metadata %#v", request)
	}
	if request.Input["duration"] != float64(5) || request.Input["resolution"] != "720p" {
		t.Fatalf("expected defaults, got %#v", request.Input)
	}
	if request.Input["cameraFixed"] != false || request.Input["seed"] != 0 {
		t.Fatalf("expected false and 0 preserved, got %#v", request.Input)
	}
}

func TestNormalizeTreatsNullAndEmptyStringAsMissingForDefaults(t *testing.T) {
	request, err := Normalize("video", testSchema(), map[string]any{
		"prompt":     "lake",
		"image":      "lake.png",
		"duration":   nil,
		"resolution": "",
	})

	if err != nil {
		t.Fatalf("expected normalized request, got %v", err)
	}
	if request.Input["duration"] != float64(5) || request.Input["resolution"] != "720p" {
		t.Fatalf("expected defaults for null and empty string, got %#v", request.Input)
	}
}

func TestNormalizeRejectsMissingRequiredField(t *testing.T) {
	_, err := Normalize("video", testSchema(), map[string]any{"image": "lake.png"})

	assertValidationDetail(t, err, errdefs.CodeMissingRequiredField, "prompt")
}

func TestNormalizeRejectsUnknownFieldByDefault(t *testing.T) {
	_, err := Normalize("video", testSchema(), map[string]any{
		"prompt": "lake",
		"image":  "lake.png",
		"extra":  true,
	})

	assertValidationDetail(t, err, errdefs.CodeUnknownField, "extra")
}

func TestNormalizeStripsUnknownFieldWhenPolicyIsStrip(t *testing.T) {
	s := testSchema()
	s.Input["unknownPolicy"] = "strip"

	request, err := Normalize("video", s, map[string]any{
		"prompt": "lake",
		"image":  "lake.png",
		"extra":  true,
	})

	if err != nil {
		t.Fatalf("expected normalized request, got %v", err)
	}
	if _, ok := request.Input["extra"]; ok {
		t.Fatalf("expected extra field stripped, got %#v", request.Input)
	}
}

func TestNormalizePassesThroughUnknownFieldWhenPolicyAllows(t *testing.T) {
	s := testSchema()
	s.Input["unknownPolicy"] = "passthrough"

	request, err := Normalize("video", s, map[string]any{
		"prompt": "lake",
		"image":  "lake.png",
		"extra":  true,
	})

	if err != nil {
		t.Fatalf("expected normalized request, got %v", err)
	}
	if request.Input["extra"] != true {
		t.Fatalf("expected extra field passthrough, got %#v", request.Input)
	}
}

func TestNormalizeRejectsInvalidTypeEnumRangeAndLength(t *testing.T) {
	for _, tt := range []struct {
		name  string
		raw   map[string]any
		code  string
		field string
	}{
		{name: "type", raw: map[string]any{"prompt": 42, "image": "lake.png"}, code: errdefs.CodeInvalidFieldType, field: "prompt"},
		{name: "enum", raw: map[string]any{"prompt": "lake", "image": "lake.png", "duration": 7}, code: errdefs.CodeInvalidEnumValue, field: "duration"},
		{name: "range", raw: map[string]any{"prompt": "lake", "image": "lake.png", "seed": -1}, code: errdefs.CodeInvalidRange, field: "seed"},
		{name: "length", raw: map[string]any{"prompt": "", "image": "lake.png"}, code: errdefs.CodeMissingRequiredField, field: "prompt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Normalize("video", testSchema(), tt.raw)
			assertValidationDetail(t, err, tt.code, tt.field)
		})
	}
}

func TestNormalizeRejectsCommandArtifactMismatch(t *testing.T) {
	_, err := Normalize("image", testSchema(), map[string]any{"prompt": "lake", "image": "lake.png"})

	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeCommandTypeMismatch {
		t.Fatalf("expected %s, got %q", errdefs.CodeCommandTypeMismatch, appErr.Code)
	}
}

func assertValidationDetail(t *testing.T, err error, code string, field string) {
	t.Helper()
	var appErr apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T %[1]v", err)
	}
	if appErr.Code != errdefs.CodeValidationError {
		t.Fatalf("expected %s, got %q", errdefs.CodeValidationError, appErr.Code)
	}
	details, ok := appErr.Details.([]FieldError)
	if !ok {
		t.Fatalf("expected []FieldError details, got %#v", appErr.Details)
	}
	for _, detail := range details {
		if detail.Code == code && detail.Field == field {
			return
		}
	}
	t.Fatalf("expected detail code=%s field=%s, got %#v", code, field, details)
}

func testSchema() schema.Schema {
	return schema.Schema{
		Provider:      "seeddance",
		Model:         "v1",
		Type:          "image-to-video",
		SchemaVersion: "1.0",
		ArtifactKind:  "video",
		Input: map[string]any{
			"unknownPolicy": "reject",
			"required":      []any{"prompt", "image"},
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":      "string",
					"minLength": float64(1),
				},
				"image": map[string]any{
					"type": "file",
				},
				"duration": map[string]any{
					"type":    "number",
					"default": float64(5),
					"enum":    []any{float64(5), float64(10)},
				},
				"resolution": map[string]any{
					"type":    "string",
					"default": "720p",
					"enum":    []any{"720p", "1080p"},
				},
				"cameraFixed": map[string]any{
					"type": "boolean",
				},
				"seed": map[string]any{
					"type":    "integer",
					"minimum": float64(0),
				},
			},
		},
	}
}
