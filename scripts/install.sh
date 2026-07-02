#!/bin/sh
set -eu

SERVER_URL="${PI_PRO_SERVER_URL:-https://api.pi-pro.org}"
PI_PRO_HOME="${PI_PRO_HOME:-$HOME/.pi-pro}"
CHANNEL="${PI_PRO_CHANNEL:-internal}"
DRY_RUN="${PI_PRO_DRY_RUN:-0}"

detect_os() {
  if [ -n "${PI_PRO_OS:-}" ]; then
    printf '%s\n' "$PI_PRO_OS"
    return
  fi
  case "$(uname -s)" in
    Darwin) printf '%s\n' "darwin" ;;
    Linux) printf '%s\n' "linux" ;;
    MINGW*|MSYS*|CYGWIN*) printf '%s\n' "windows" ;;
    *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
}

detect_arch() {
  if [ -n "${PI_PRO_ARCH:-}" ]; then
    printf '%s\n' "$PI_PRO_ARCH"
    return
  fi
  case "$(uname -m)" in
    x86_64|amd64) printf '%s\n' "amd64" ;;
    arm64|aarch64) printf '%s\n' "arm64" ;;
    *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
  esac
}

need_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

json_string_field() {
  field="$1"
  sed -n "s/.*\"$field\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p"
}

sha256_file() {
  file="$1"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
    return
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return
  fi
  if command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$file" | awk '{print $NF}'
    return
  fi
  echo "missing sha256 tool: install shasum, sha256sum, or openssl" >&2
  exit 1
}

download() {
  url="$1"
  out="$2"
  curl -fsSL -o "$out" "$url"
}

profile_file() {
  if [ -n "${PI_PRO_INSTALL_PROFILE:-}" ]; then
    printf '%s\n' "$PI_PRO_INSTALL_PROFILE"
    return
  fi
  case "${SHELL:-}" in
    */zsh) printf '%s\n' "$HOME/.zshrc" ;;
    */bash) printf '%s\n' "$HOME/.bashrc" ;;
    *) printf '%s\n' "$HOME/.profile" ;;
  esac
}

ensure_path() {
  dir="$1"
  export PATH="$dir:$PATH"

  profile="$(profile_file)"
  marker="# pi-pro PATH"
  line="export PATH=\"$dir:\$PATH\""
  if [ ! -f "$profile" ] || ! grep -F "$line" "$profile" >/dev/null 2>&1; then
    {
      printf '\n%s\n' "$marker"
      printf '%s\n' "$line"
    } >> "$profile"
  fi
  # shellcheck disable=SC1090
  . "$profile" >/dev/null 2>&1 || true
  echo "PATH profile updated: $profile"
}

need_command curl
need_command awk
need_command sed

os_name="$(detect_os)"
arch_name="$(detect_arch)"
bin_name="pi-pro"
if [ "$os_name" = "windows" ]; then
  bin_name="pi-pro.exe"
fi

request="{\"localVersion\":\"none\",\"os\":\"$os_name\",\"arch\":\"$arch_name\",\"channel\":\"$CHANNEL\"}"
version_url="${SERVER_URL%/}/cli/version"
version_response="$(curl -fsSL -X POST -H "Content-Type: application/json" -d "$request" "$version_url")"

binary_url="$(printf '%s\n' "$version_response" | json_string_field "url" | head -n 1)"
binary_sha="$(printf '%s\n' "$version_response" | json_string_field "sha256" | head -n 1)"
release_version="$(printf '%s\n' "$version_response" | json_string_field "releaseVersion" | head -n 1)"

if [ -z "$binary_url" ] || [ -z "$binary_sha" ]; then
  echo "version response missing binary url or sha256" >&2
  exit 1
fi

bin_dir="$PI_PRO_HOME/bin"
target="$bin_dir/$bin_name"

echo "pi-pro installer"
echo "platform: $os_name/$arch_name"
if [ -n "$release_version" ]; then
  echo "release: $release_version"
fi
echo "install path: $target"

if [ "$DRY_RUN" = "1" ] || [ "$DRY_RUN" = "true" ]; then
  echo "dry-run: binary download and install skipped"
  echo "After installation, run: pi-pro init"
  exit 0
fi

mkdir -p "$bin_dir"
tmp_file="$bin_dir/.$bin_name.download"
trap 'rm -f "$tmp_file"' EXIT INT TERM

download "$binary_url" "$tmp_file"
actual_sha="$(sha256_file "$tmp_file")"
if [ "$actual_sha" != "$binary_sha" ]; then
  echo "checksum mismatch for downloaded binary" >&2
  echo "expected: $binary_sha" >&2
  echo "actual:   $actual_sha" >&2
  exit 1
fi

chmod 755 "$tmp_file"
mv "$tmp_file" "$target"
trap - EXIT INT TERM

ensure_path "$bin_dir"

echo "installed: $target"
echo "pi-pro is available in new shell sessions."
echo "After installation, run: pi-pro init"
