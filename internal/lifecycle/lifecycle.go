package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/a754962942/pi-pro-cli/internal/apperror"
	"github.com/a754962942/pi-pro-cli/internal/assets"
	"github.com/a754962942/pi-pro-cli/internal/client"
	"github.com/a754962942/pi-pro-cli/internal/config"
	"github.com/a754962942/pi-pro-cli/internal/errdefs"
	"github.com/a754962942/pi-pro-cli/internal/serverapi"
	"github.com/a754962942/pi-pro-cli/internal/updater"
)

type HTTPClient = client.HTTPClient

type Options struct {
	ConfigDir       string
	ServerURL       string
	LocalVersion    string
	HTTPClient      client.HTTPClient
	ExecutablePath  string
	OperatingSystem string
}

type VersionCheck struct {
	LocalVersion        string `json:"localVersion"`
	ReleaseVersion      string `json:"releaseVersion"`
	MinSupportedVersion string `json:"minSupportedVersion,omitempty"`
	UpdateAvailable     bool   `json:"updateAvailable"`
	UpdateRequired      bool   `json:"updateRequired"`
	Binary              Binary `json:"binary,omitempty"`
	InitManifestVersion string `json:"initManifestVersion,omitempty"`
}

type Binary struct {
	URL    string `json:"url,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type InitResult struct {
	OK              bool          `json:"ok"`
	Initialized     bool          `json:"initialized"`
	Changed         bool          `json:"changed"`
	ConfigDir       string        `json:"configDir"`
	ManifestVersion string        `json:"manifestVersion"`
	Files           FileStats     `json:"files"`
	Version         VersionResult `json:"version"`
}

type FileStats struct {
	Downloaded int `json:"downloaded"`
	Skipped    int `json:"skipped"`
}

type VersionResult struct {
	Checked        bool   `json:"checked"`
	Updated        bool   `json:"updated"`
	LocalVersion   string `json:"localVersion"`
	ReleaseVersion string `json:"releaseVersion"`
}

type UpdateResult struct {
	OK             bool   `json:"ok"`
	Changed        bool   `json:"changed"`
	LocalVersion   string `json:"localVersion"`
	ReleaseVersion string `json:"releaseVersion"`
}

type VersionState struct {
	CheckedAt           string `json:"checkedAt"`
	LocalVersion        string `json:"localVersion"`
	ReleaseVersion      string `json:"releaseVersion"`
	MinSupportedVersion string `json:"minSupportedVersion,omitempty"`
	UpdateAvailable     bool   `json:"updateAvailable"`
	UpdateRequired      bool   `json:"updateRequired"`
}

type manifest struct {
	Version string         `json:"version"`
	Files   []manifestFile `json:"files"`
	SQLite  struct {
		AssetDBSchemaVersion int `json:"assetDbSchemaVersion"`
	} `json:"sqlite"`
}

type manifestFile struct {
	Path     string `json:"path"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Required bool   `json:"required"`
}

func Init(ctx context.Context, options Options) (InitResult, error) {
	options = withDefaults(options)
	paths := config.PathsFor(options.ConfigDir)
	if err := config.EnsureDirs(paths); err != nil {
		return InitResult{}, err
	}

	versionCheck, err := checkVersion(ctx, options)
	if err != nil {
		return InitResult{}, err
	}
	if err := writeVersionState(paths.VersionState, versionCheck); err != nil {
		return InitResult{}, err
	}
	if versionCheck.LocalVersion != versionCheck.ReleaseVersion || versionCheck.UpdateRequired {
		return InitResult{}, updateRequired(versionCheck)
	}

	manifest, err := fetchManifest(ctx, options)
	if err != nil {
		return InitResult{}, err
	}
	if err := validateManifest(manifest); err != nil {
		return InitResult{}, err
	}

	result := InitResult{
		OK:              true,
		Initialized:     true,
		ConfigDir:       options.ConfigDir,
		ManifestVersion: manifest.Version,
		Version: VersionResult{
			Checked:        true,
			Updated:        false,
			LocalVersion:   versionCheck.LocalVersion,
			ReleaseVersion: versionCheck.ReleaseVersion,
		},
	}

	for _, file := range manifest.Files {
		target, err := safeTarget(paths.ConfigDir, file.Path)
		if err != nil {
			return InitResult{}, apperror.AppError{Code: errdefs.CodeInitPathInvalid, Message: "init manifest contains invalid file path", Kind: apperror.KindLifecycle, Details: map[string]any{"path": file.Path}}
		}
		if matchesChecksum(target, file.SHA256) {
			result.Files.Skipped++
			continue
		}

		data, err := download(ctx, options, file.URL)
		if err != nil {
			return InitResult{}, err
		}
		if sha256Hex(data) != file.SHA256 {
			return InitResult{}, apperror.AppError{Code: errdefs.CodeInitFileChecksumMismatch, Message: "downloaded init file checksum did not match manifest", Kind: apperror.KindLifecycle, Details: map[string]any{"path": file.Path}}
		}
		if err := atomicWrite(target, data, 0o600); err != nil {
			return InitResult{}, err
		}
		result.Files.Downloaded++
		result.Changed = true
	}

	if err := initAssetDB(paths.AssetDB); err != nil {
		return InitResult{}, err
	}
	if err := writeInitState(paths.InitState, result); err != nil {
		return InitResult{}, err
	}
	return result, nil
}

func Update(ctx context.Context, options Options) (UpdateResult, error) {
	options = withDefaults(options)
	paths := config.PathsFor(options.ConfigDir)
	if err := config.EnsureDirs(paths); err != nil {
		return UpdateResult{}, err
	}
	versionCheck, err := checkVersion(ctx, options)
	if err != nil {
		return UpdateResult{}, err
	}
	if err := writeVersionState(paths.VersionState, versionCheck); err != nil {
		return UpdateResult{}, err
	}
	changed := versionCheck.LocalVersion != versionCheck.ReleaseVersion
	if changed {
		if versionCheck.Binary.URL == "" || versionCheck.Binary.SHA256 == "" {
			return UpdateResult{}, apperror.AppError{Code: errdefs.CodeServerResponseInvalid, Message: "version response missing binary update metadata", Kind: apperror.KindNetwork}
		}
		currentExecutable, err := executablePath(options)
		if err != nil {
			return UpdateResult{}, err
		}
		if err := updater.ValidateManagedExecutable(currentExecutable, paths.BinFile); err != nil {
			return UpdateResult{}, err
		}
		data, err := downloadWithError(ctx, options, versionCheck.Binary.URL, errdefs.CodeUpdateDownloadFailed, "failed to download update binary")
		if err != nil {
			return UpdateResult{}, err
		}
		if sha256Hex(data) != versionCheck.Binary.SHA256 {
			return UpdateResult{}, apperror.AppError{Code: errdefs.CodeUpdateChecksumMismatch, Message: "downloaded update binary checksum did not match version response", Kind: apperror.KindLifecycle}
		}
		stagedPath, err := stageUpdateBinary(paths.ConfigDir, versionCheck.ReleaseVersion, data)
		if err != nil {
			return UpdateResult{}, err
		}
		if err := updater.Apply(updater.ApplyRequest{
			CurrentExecutablePath: currentExecutable,
			ManagedBinaryPath:     paths.BinFile,
			StagedBinaryPath:      stagedPath,
			HelperPath:            filepath.Join(paths.BinDir, "pi-pro-updater.exe"),
			ManagedRootPath:       paths.ConfigDir,
			ExpectedSHA256:        versionCheck.Binary.SHA256,
			TargetVersion:         versionCheck.ReleaseVersion,
			ParentPID:             os.Getpid(),
			GOOS:                  operatingSystem(options),
		}); err != nil {
			return UpdateResult{}, err
		}
		if operatingSystem(options) != "windows" && !matchesChecksum(paths.BinFile, versionCheck.Binary.SHA256) {
			return UpdateResult{}, apperror.AppError{Code: errdefs.CodeUpdateReplaceFailed, Message: "installed update binary checksum did not match version response", Kind: apperror.KindLifecycle}
		}
	}
	return UpdateResult{
		OK:             true,
		Changed:        changed,
		LocalVersion:   versionCheck.LocalVersion,
		ReleaseVersion: versionCheck.ReleaseVersion,
	}, nil
}

func checkVersion(ctx context.Context, options Options) (VersionCheck, error) {
	api := client.New(client.Config{ServerURL: options.ServerURL, HTTPClient: options.HTTPClient})
	var response VersionCheck
	err := api.DoJSON(ctx, http.MethodPost, serverapi.CLIVersion, map[string]string{
		"localVersion": options.LocalVersion,
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"channel":      "internal",
	}, &response)
	if err != nil {
		return VersionCheck{}, err
	}
	if response.LocalVersion == "" {
		response.LocalVersion = options.LocalVersion
	}
	if response.ReleaseVersion == "" {
		return VersionCheck{}, apperror.AppError{Code: errdefs.CodeServerResponseInvalid, Message: "version response missing releaseVersion", Kind: apperror.KindNetwork}
	}
	response.UpdateRequired = response.UpdateRequired || response.LocalVersion != response.ReleaseVersion
	response.UpdateAvailable = response.UpdateAvailable || response.LocalVersion != response.ReleaseVersion
	return response, nil
}

func fetchManifest(ctx context.Context, options Options) (manifest, error) {
	api := client.New(client.Config{ServerURL: options.ServerURL, HTTPClient: options.HTTPClient})
	var response manifest
	if err := api.DoJSON(ctx, http.MethodGet, serverapi.CLIInitManifest, nil, &response); err != nil {
		return manifest{}, err
	}
	return response, nil
}

func validateManifest(manifest manifest) error {
	if manifest.Version == "" {
		return invalidManifest("manifest version is required", "")
	}
	for _, file := range manifest.Files {
		if file.Path == "" {
			return invalidManifest("manifest file path is required", file.Path)
		}
		if file.URL == "" {
			return invalidManifest("manifest file url is required", file.Path)
		}
		if file.SHA256 == "" {
			return invalidManifest("manifest file sha256 is required", file.Path)
		}
	}
	return nil
}

func invalidManifest(message string, path string) error {
	details := map[string]any{}
	if path != "" {
		details["path"] = path
	}
	return apperror.AppError{Code: errdefs.CodeInitManifestInvalid, Message: message, Kind: apperror.KindLifecycle, Details: details}
}

func download(ctx context.Context, options Options, fileURL string) ([]byte, error) {
	return downloadWithError(ctx, options, fileURL, errdefs.CodeInitFileDownloadFailed, "failed to download init file")
}

func downloadWithError(ctx context.Context, options Options, fileURL string, code string, message string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolveURL(options.ServerURL, fileURL), nil)
	if err != nil {
		return nil, err
	}
	resp, err := options.HTTPClient.Do(req)
	if err != nil {
		return nil, apperror.AppError{Code: code, Message: message, Kind: apperror.KindLifecycle, Details: map[string]any{"error": err.Error()}}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, apperror.AppError{Code: code, Message: message, Kind: apperror.KindLifecycle, Details: map[string]any{"statusCode": resp.StatusCode}}
	}
	return io.ReadAll(resp.Body)
}

func safeTarget(configDir string, relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("invalid relative path")
	}
	cleaned := filepath.Clean(relativePath)
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("path traversal")
	}
	target := filepath.Join(configDir, cleaned)
	if !strings.HasPrefix(target, filepath.Clean(configDir)+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes config dir")
	}
	return target, nil
}

func matchesChecksum(path string, expected string) bool {
	if expected == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return sha256Hex(data) == expected
}

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp-%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func initAssetDB(path string) error {
	store, err := assets.Open(path)
	if err != nil {
		return err
	}
	return store.Close()
}

func writeInitState(path string, result InitResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(path, data, 0o600)
}

func writeVersionState(path string, versionCheck VersionCheck) error {
	state := VersionState{
		CheckedAt:           time.Now().UTC().Format(time.RFC3339),
		LocalVersion:        versionCheck.LocalVersion,
		ReleaseVersion:      versionCheck.ReleaseVersion,
		MinSupportedVersion: versionCheck.MinSupportedVersion,
		UpdateAvailable:     versionCheck.UpdateAvailable,
		UpdateRequired:      versionCheck.UpdateRequired,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(path, data, 0o600)
}

func stageUpdateBinary(configDir string, releaseVersion string, data []byte) (string, error) {
	filename := "pi-pro-" + releaseVersion
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	path := filepath.Join(configDir, "updates", filename)
	if err := atomicWrite(path, data, 0o755); err != nil {
		return "", apperror.AppError{Code: errdefs.CodeUpdateReplaceFailed, Message: "failed to stage update binary", Kind: apperror.KindLifecycle, Details: map[string]any{"error": err.Error()}}
	}
	return path, nil
}

func executablePath(options Options) (string, error) {
	if options.ExecutablePath != "" {
		return options.ExecutablePath, nil
	}
	path, err := os.Executable()
	if err != nil {
		return "", apperror.AppError{Code: errdefs.CodeUpdateUnsupportedInstallLocation, Message: "failed to resolve current executable path", Kind: apperror.KindLifecycle, Details: map[string]any{"error": err.Error()}}
	}
	return path, nil
}

func operatingSystem(options Options) string {
	if options.OperatingSystem != "" {
		return options.OperatingSystem
	}
	return runtime.GOOS
}

func updateRequired(versionCheck VersionCheck) error {
	return apperror.AppError{
		Code:    errdefs.CodeUpdateRequired,
		Message: "CLI version is outdated. Please run `pi-pro update` before continuing.",
		Kind:    apperror.KindLifecycle,
		Details: map[string]any{
			"localVersion":   versionCheck.LocalVersion,
			"releaseVersion": versionCheck.ReleaseVersion,
		},
	}
}

func withDefaults(options Options) Options {
	if options.ConfigDir == "" {
		if configDir, err := config.ResolveConfigDir(); err == nil {
			options.ConfigDir = configDir
		}
	}
	if options.ServerURL == "" {
		options.ServerURL = config.Runtime().ServerURL
	}
	if options.LocalVersion == "" {
		options.LocalVersion = config.LocalVersion
	}
	if options.HTTPClient == nil {
		options.HTTPClient = http.DefaultClient
	}
	return options
}

func resolveURL(serverURL string, value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return strings.TrimRight(serverURL, "/") + "/" + strings.TrimLeft(value, "/")
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
