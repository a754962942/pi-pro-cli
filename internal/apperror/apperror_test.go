package apperror

import "testing"

func TestAppErrorImplementsError(t *testing.T) {
	err := AppError{Code: "TEST_ERROR", Message: "test message", Kind: KindValidation}

	if err.Error() != "test message" {
		t.Fatalf("expected Error to return message, got %q", err.Error())
	}
}

func TestConstructorsSetKind(t *testing.T) {
	usage := Usage("USAGE_ERROR", "bad usage")
	if usage.Kind != KindUsage {
		t.Fatalf("expected usage kind, got %q", usage.Kind)
	}

	notImplemented := NotImplemented("NOT_IMPLEMENTED", "not implemented")
	if notImplemented.Kind != KindNotImplemented {
		t.Fatalf("expected not implemented kind, got %q", notImplemented.Kind)
	}
}
