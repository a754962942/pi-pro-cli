package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalVersionHasDevelopmentDefault(t *testing.T) {
	if LocalVersion != "0.0.0-dev" {
		t.Fatalf("expected default local version 0.0.0-dev, got %q", LocalVersion)
	}
}

func TestLocalVersionCanBeInjectedAtBuildTime(t *testing.T) {
	original := LocalVersion
	t.Cleanup(func() {
		LocalVersion = original
	})

	LocalVersion = "1.2.3-test"

	if LocalVersion != "1.2.3-test" {
		t.Fatalf("expected local version to be injectable, got %q", LocalVersion)
	}
}

func TestResolveConfigDirUsesEnvironmentOverride(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "pi-pro-config")
	t.Setenv(EnvConfigDir, configDir)

	got, err := ResolveConfigDir()
	if err != nil {
		t.Fatalf("expected config dir, got error %v", err)
	}

	if got != configDir {
		t.Fatalf("expected env config dir %q, got %q", configDir, got)
	}
}

func TestDefaultConfigDirForHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")

	got := DefaultConfigDirForHome(home)

	want := filepath.Join(home, ".pi-pro")
	if got != want {
		t.Fatalf("expected default config dir %q, got %q", want, got)
	}
}

func TestPathsForReturnsLocalStatePaths(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")

	paths := PathsFor(configDir)

	want := map[string]string{
		"ConfigDir":    configDir,
		"ConfigFile":   filepath.Join(configDir, "config.json"),
		"AssetDB":      filepath.Join(configDir, "assets.sqlite"),
		"InitState":    filepath.Join(configDir, "init-state.json"),
		"VersionState": filepath.Join(configDir, "version-state.json"),
		"SchemasDir":   filepath.Join(configDir, "schemas"),
		"BinDir":       filepath.Join(configDir, "bin"),
		"BinFile":      filepath.Join(configDir, "bin", "pi-pro"),
	}
	got := map[string]string{
		"ConfigDir":    paths.ConfigDir,
		"ConfigFile":   paths.ConfigFile,
		"AssetDB":      paths.AssetDB,
		"InitState":    paths.InitState,
		"VersionState": paths.VersionState,
		"SchemasDir":   paths.SchemasDir,
		"BinDir":       paths.BinDir,
		"BinFile":      paths.BinFile,
	}

	for key, expected := range want {
		if got[key] != expected {
			t.Fatalf("expected %s=%q, got %q", key, expected, got[key])
		}
	}
}

func TestBinFileNameForOS(t *testing.T) {
	if got := BinFileNameForOS("windows"); got != "pi-pro.exe" {
		t.Fatalf("expected Windows binary name pi-pro.exe, got %q", got)
	}
	if got := BinFileNameForOS("darwin"); got != "pi-pro" {
		t.Fatalf("expected Unix-like binary name pi-pro, got %q", got)
	}
}

func TestEnsureDirsCreatesSafeDirectories(t *testing.T) {
	paths := PathsFor(filepath.Join(t.TempDir(), "config"))

	if err := EnsureDirs(paths); err != nil {
		t.Fatalf("expected dirs to be created, got %v", err)
	}

	for _, dir := range []string{paths.ConfigDir, paths.SchemasDir, paths.BinDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected dir %s to exist, got %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a dir", dir)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("expected %s permissions 0700, got %o", dir, got)
		}
	}
}

func TestLoadMissingConfigReturnsEmptyConfig(t *testing.T) {
	paths := PathsFor(filepath.Join(t.TempDir(), "config"))

	cfg, err := Load(paths)
	if err != nil {
		t.Fatalf("expected missing config to load as empty, got %v", err)
	}

	if cfg.AuthToken != "" || cfg.Username != "" {
		t.Fatalf("expected empty config, got %#v", cfg)
	}
}

func TestSaveAndLoadConfigUsesSafeFilePermissions(t *testing.T) {
	paths := PathsFor(filepath.Join(t.TempDir(), "config"))
	cfg := File{
		AuthToken: "sk-pipro-testtoken",
		Username:  "user@example.com",
	}

	if err := Save(paths, cfg); err != nil {
		t.Fatalf("expected config save, got %v", err)
	}

	info, err := os.Stat(paths.ConfigFile)
	if err != nil {
		t.Fatalf("expected config file to exist, got %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected config file permissions 0600, got %o", got)
	}

	loaded, err := Load(paths)
	if err != nil {
		t.Fatalf("expected config load, got %v", err)
	}
	if loaded != cfg {
		t.Fatalf("expected loaded config %#v, got %#v", cfg, loaded)
	}
}

func TestRuntimeConfigUsesBuiltInServerURL(t *testing.T) {
	runtime := Runtime()

	if runtime.ServerURL != BuiltInServerURL {
		t.Fatalf("expected built-in server URL %q, got %q", BuiltInServerURL, runtime.ServerURL)
	}
}
