package serverapi

import "strings"

const (
	AuthLogin        = "/auth/login"
	CLIVersion       = "/cli/version"
	CLIInitManifest  = "/cli/init-manifest"
	CLIFilesPrefix   = "/cli/files/"
	CLISchema        = "/cli/schema"
	CLIReleaseBinary = "/cli/releases/pi-pro"
	CapabilityTypes  = "/capabilities/types"
	Generations      = "/generations"
	AssetsUpload     = "/assets/upload"
)

func TaskStatus(jobID string) string {
	return "/tasks/" + strings.TrimLeft(jobID, "/")
}

func TaskCancel(jobID string) string {
	return TaskStatus(jobID) + "/cancel"
}

func CapabilityModels(eventType string) string {
	return "/capabilities/types/" + strings.TrimLeft(eventType, "/") + "/models"
}
