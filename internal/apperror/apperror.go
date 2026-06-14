package apperror

type Kind string

const (
	KindUsage          Kind = "usage"
	KindValidation     Kind = "validation"
	KindAuth           Kind = "auth"
	KindNetwork        Kind = "network"
	KindTask           Kind = "task"
	KindIO             Kind = "io"
	KindLifecycle      Kind = "lifecycle"
	KindNotImplemented Kind = "not_implemented"
)

type AppError struct {
	Code    string
	Message string
	Kind    Kind
	Details any
}

func (e AppError) Error() string {
	return e.Message
}

func Usage(code string, message string) AppError {
	return AppError{Code: code, Message: message, Kind: KindUsage}
}

func NotImplemented(code string, message string) AppError {
	return AppError{Code: code, Message: message, Kind: KindNotImplemented}
}
