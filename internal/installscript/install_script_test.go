package installscript

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptInstallsBinaryOnlyAndPrintsInitNextStep(t *testing.T) {
	binary := []byte("#!/bin/sh\necho pi-pro test\n")
	home := t.TempDir()
	fixture := installFixture(t, binary, sha256Hex(binary))

	result := runInstallScript(t, home, fixture, nil)

	if result.err != nil {
		t.Fatalf("expected install success, got %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	installed := filepath.Join(home, "bin", "pi-pro")
	data, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("expected installed binary, got %v", err)
	}
	if string(data) != string(binary) {
		t.Fatalf("unexpected installed binary %q", string(data))
	}
	for _, path := range []string{
		filepath.Join(home, "schemas"),
		filepath.Join(home, "assets.sqlite"),
		filepath.Join(home, "config.json"),
		filepath.Join(home, "init-state.json"),
	} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("install.sh must not create runtime file %s", path)
		}
	}
	if !strings.Contains(result.stdout, "pi-pro init") {
		t.Fatalf("expected init next step, got stdout=%s", result.stdout)
	}
}

func TestInstallScriptSendsNoLocalVersionRequest(t *testing.T) {
	binary := []byte("binary")
	fixture := installFixture(t, binary, sha256Hex(binary))

	result := runInstallScript(t, t.TempDir(), fixture, nil)

	if result.err != nil {
		t.Fatalf("expected install success, got %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	body, err := os.ReadFile(filepath.Join(fixture, "version-request.json"))
	if err != nil {
		t.Fatalf("expected captured version request, got %v", err)
	}
	text := string(body)
	for _, expected := range []string{`"localVersion":"none"`, `"os":"darwin"`, `"arch":"arm64"`, `"channel":"internal"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected request to contain %s, got %s", expected, text)
		}
	}
}

func TestInstallScriptDefaultsToProductionServerURL(t *testing.T) {
	binary := []byte("binary")
	fixture := installFixture(t, binary, sha256Hex(binary))

	result := runInstallScript(t, t.TempDir(), fixture, nil)

	if result.err != nil {
		t.Fatalf("expected install success, got %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertVersionURL(t, fixture, "https://api.pi-pro.org/cli/version")
}

func TestInstallScriptAllowsServerURLOverride(t *testing.T) {
	binary := []byte("binary")
	fixture := installFixture(t, binary, sha256Hex(binary))

	result := runInstallScript(t, t.TempDir(), fixture, map[string]string{"PI_PRO_SERVER_URL": "https://api.example.test"})

	if result.err != nil {
		t.Fatalf("expected install success, got %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	assertVersionURL(t, fixture, "https://api.example.test/cli/version")
}

func TestInstallScriptRejectsChecksumMismatch(t *testing.T) {
	fixture := installFixture(t, []byte("binary"), strings.Repeat("0", 64))
	home := t.TempDir()

	result := runInstallScript(t, home, fixture, nil)

	if result.err == nil {
		t.Fatalf("expected checksum failure")
	}
	if !strings.Contains(result.stderr, "checksum mismatch") {
		t.Fatalf("expected checksum mismatch stderr, got %q", result.stderr)
	}
	if _, err := os.Stat(filepath.Join(home, "bin", "pi-pro")); err == nil {
		t.Fatalf("binary should not be installed on checksum mismatch")
	}
}

func TestInstallScriptDryRunDoesNotInstallBinary(t *testing.T) {
	binary := []byte("binary")
	fixture := installFixture(t, binary, sha256Hex(binary))
	home := t.TempDir()

	result := runInstallScript(t, home, fixture, map[string]string{"PI_PRO_DRY_RUN": "1"})

	if result.err != nil {
		t.Fatalf("expected dry-run success, got %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	if _, err := os.Stat(filepath.Join(home, "bin", "pi-pro")); err == nil {
		t.Fatalf("dry-run should not install binary")
	}
	if !strings.Contains(result.stdout, "dry-run") {
		t.Fatalf("expected dry-run output, got %q", result.stdout)
	}
}

type installResult struct {
	stdout string
	stderr string
	err    error
}

func runInstallScript(t *testing.T, home string, fixture string, extra map[string]string) installResult {
	t.Helper()
	root := repoRoot(t)
	fakeBin := fakeCurlBin(t, fixture)
	cmd := exec.Command("sh", filepath.Join(root, "scripts", "install.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PI_PRO_HOME="+home,
		"PI_PRO_OS=darwin",
		"PI_PRO_ARCH=arm64",
	)
	for key, value := range extra {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	stdout, stderr := strings.Builder{}, strings.Builder{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return installResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("expected cwd, got %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func installFixture(t *testing.T, binary []byte, checksum string) string {
	t.Helper()
	dir := t.TempDir()
	binaryURL := "https://download.example/pi-pro"
	version := fmt.Sprintf(`{"releaseVersion":"0.1.0","binary":{"url":"%s","sha256":"%s"}}`, binaryURL, checksum)
	if err := os.WriteFile(filepath.Join(dir, "version.json"), []byte(version), 0o600); err != nil {
		t.Fatalf("expected version fixture write, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pi-pro"), binary, 0o600); err != nil {
		t.Fatalf("expected binary fixture write, got %v", err)
	}
	return dir
}

func assertVersionURL(t *testing.T, fixture string, expected string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixture, "version-url.txt"))
	if err != nil {
		t.Fatalf("expected captured version URL, got %v", err)
	}
	if actual := strings.TrimSpace(string(data)); actual != expected {
		t.Fatalf("expected version URL %q, got %q", expected, actual)
	}
}

func fakeCurlBin(t *testing.T, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "curl")
	body := `#!/bin/sh
set -eu
fixture="` + fixture + `"
out=""
data=""
last=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      shift
      out="$1"
      ;;
    -d|--data|--data-raw)
      shift
      data="$1"
      ;;
  esac
  last="$1"
  shift
done
if [ -n "$data" ]; then
  printf '%s' "$data" > "$fixture/version-request.json"
fi
case "$last" in
  */cli/version)
    printf '%s\n' "$last" > "$fixture/version-url.txt"
    cat "$fixture/version.json"
    ;;
  https://download.example/pi-pro)
    if [ -n "$out" ]; then
      cp "$fixture/pi-pro" "$out"
    else
      cat "$fixture/pi-pro"
    fi
    ;;
  *)
    echo "unexpected curl url: $last" >&2
    exit 22
    ;;
esac
`
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatalf("expected fake curl write, got %v", err)
	}
	return dir
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
