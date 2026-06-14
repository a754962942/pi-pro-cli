package serverapi

import "strings"

const (
	AuthLogin        = "/auth/login"
	CLIVersion       = "/cli/version"
	CLIInitManifest  = "/cli/init-manifest"
	CLIFilesPrefix   = "/cli/files/"
	CLIReleaseBinary = "/cli/releases/pi-pro"
	Generations      = "/generations"
	AssetsUpload     = "/assets/upload"
)

func TaskStatus(jobID string) string {
	return "/tasks/" + strings.TrimLeft(jobID, "/")
}

func TaskCancel(jobID string) string {
	return TaskStatus(jobID) + "/cancel"
}
