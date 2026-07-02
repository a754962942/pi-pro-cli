package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const EnvConfigDir = "PI_PRO_CONFIG_DIR"
const EnvServerURL = "PI_PRO_SERVER_URL"

// BuiltInServerURL is injected by release builds with:
// go build -ldflags "-X github.com/a754962942/pi-pro-cli/internal/config.BuiltInServerURL=<server-url>"
var BuiltInServerURL = "https://api.example.com"

// LocalVersion is injected by release builds with:
// go build -ldflags "-X github.com/a754962942/pi-pro-cli/internal/config.LocalVersion=<version>"
var LocalVersion = "0.0.0-dev"

type Paths struct {
	ConfigDir    string
	ConfigFile   string
	AssetDB      string
	InitState    string
	VersionState string
	SchemasDir   string
	BinDir       string
	BinFile      string
}

type File struct {
	AuthToken string `json:"authToken,omitempty"`
	Username  string `json:"username,omitempty"`
}

type RuntimeConfig struct {
	ServerURL string
}

func Runtime() RuntimeConfig {
	serverURL := os.Getenv(EnvServerURL)
	if serverURL == "" {
		serverURL = BuiltInServerURL
	}
	return RuntimeConfig{
		ServerURL: serverURL,
	}
}

func ResolveConfigDir() (string, error) {
	if configDir := os.Getenv(EnvConfigDir); configDir != "" {
		return configDir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return DefaultConfigDirForHome(home), nil
}

func DefaultConfigDirForHome(home string) string {
	return filepath.Join(home, ".pi-pro")
}

func PathsFor(configDir string) Paths {
	return Paths{
		ConfigDir:    configDir,
		ConfigFile:   filepath.Join(configDir, "config.json"),
		AssetDB:      filepath.Join(configDir, "assets.sqlite"),
		InitState:    filepath.Join(configDir, "init-state.json"),
		VersionState: filepath.Join(configDir, "version-state.json"),
		SchemasDir:   filepath.Join(configDir, "schemas"),
		BinDir:       filepath.Join(configDir, "bin"),
		BinFile:      filepath.Join(configDir, "bin", BinFileNameForOS(runtime.GOOS)),
	}
}

func BinFileNameForOS(goos string) string {
	if goos == "windows" {
		return "pi-pro.exe"
	}
	return "pi-pro"
}

func EnsureDirs(paths Paths) error {
	for _, dir := range []string{paths.ConfigDir, paths.SchemasDir, paths.BinDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func Load(paths Paths) (File, error) {
	data, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, nil
		}
		return File{}, err
	}

	var cfg File
	if err := json.Unmarshal(data, &cfg); err != nil {
		return File{}, err
	}
	return cfg, nil
}

func Save(paths Paths, cfg File) error {
	if err := EnsureDirs(paths); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tempFile := paths.ConfigFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tempFile, 0o600); err != nil {
		return err
	}
	return os.Rename(tempFile, paths.ConfigFile)
}
