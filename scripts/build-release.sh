#!/bin/sh
set -eu

VERSION="${PI_PRO_VERSION:-0.1.0}"
SERVER_URL="${PI_PRO_SERVER_URL:-https://api.pi-pro.org}"
OUTPUT_DIR="${PI_PRO_OUTPUT_DIR:-dist/releases}"
TARGETS="${PI_PRO_TARGETS:-darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64}"
CONFIG_PACKAGE="github.com/a754962942/pi-pro-cli/internal/config"
LDFLAGS="-s -w -X ${CONFIG_PACKAGE}.LocalVersion=${VERSION} -X ${CONFIG_PACKAGE}.BuiltInServerURL=${SERVER_URL}"

need_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
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

json_string_field() {
  field="$1"
  sed -n "s/.*\"$field\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p"
}

host_os() {
  case "$(uname -s)" in
    Darwin) printf '%s\n' "darwin" ;;
    Linux) printf '%s\n' "linux" ;;
    *) printf '%s\n' "unknown" ;;
  esac
}

host_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf '%s\n' "amd64" ;;
    arm64|aarch64) printf '%s\n' "arm64" ;;
    *) printf '%s\n' "unknown" ;;
  esac
}

need_command awk
need_command go

HOST_OS="$(host_os)"
HOST_ARCH="$(host_arch)"

mkdir -p "$OUTPUT_DIR"
manifest="$OUTPUT_DIR/manifest.json"
tmp_manifest="$manifest.tmp"

printf '{\n' > "$tmp_manifest"
printf '  "releaseVersion": "%s",\n' "$VERSION" >> "$tmp_manifest"
printf '  "minSupportedVersion": "%s",\n' "$VERSION" >> "$tmp_manifest"
printf '  "artifacts": [\n' >> "$tmp_manifest"

first=1
for target in $TARGETS; do
  os_name="${target%-*}"
  arch_name="${target#*-}"
  platform="${os_name}-${arch_name}"
  binary_name="pi-pro"
  if [ "$os_name" = "windows" ]; then
    binary_name="pi-pro.exe"
  fi

  artifact_dir="$OUTPUT_DIR/$VERSION/$platform"
  mkdir -p "$artifact_dir"

  binary_path="$artifact_dir/$binary_name"
  echo "building $binary_path"
  GOOS="$os_name" GOARCH="$arch_name" CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$binary_path" ./cmd/pi-pro
  if [ "$os_name" = "$HOST_OS" ] && [ "$arch_name" = "$HOST_ARCH" ]; then
    built_version="$("$binary_path" --version | json_string_field "localVersion" | head -n 1)"
    if [ "$built_version" != "$VERSION" ]; then
      echo "release version mismatch: binary=$built_version manifest=$VERSION" >&2
      exit 1
    fi
  fi
  binary_sha="$(sha256_file "$binary_path")"
  manifest_binary_path="$VERSION/$platform/$binary_name"

  if [ "$first" = "0" ]; then
    printf ',\n' >> "$tmp_manifest"
  fi
  first=0

  printf '    {\n' >> "$tmp_manifest"
  printf '      "os": "%s",\n' "$os_name" >> "$tmp_manifest"
  printf '      "arch": "%s",\n' "$arch_name" >> "$tmp_manifest"
  printf '      "binary": {\n' >> "$tmp_manifest"
  printf '        "path": "%s",\n' "$manifest_binary_path" >> "$tmp_manifest"
  printf '        "sha256": "%s"\n' "$binary_sha" >> "$tmp_manifest"
  printf '      }' >> "$tmp_manifest"

  if [ "$os_name" = "windows" ]; then
    updater_name="pi-pro-updater.exe"
    updater_path="$artifact_dir/$updater_name"
    echo "building $updater_path"
    GOOS="$os_name" GOARCH="$arch_name" CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$updater_path" ./cmd/pi-pro-updater
    updater_sha="$(sha256_file "$updater_path")"
    manifest_updater_path="$VERSION/$platform/$updater_name"
    printf ',\n' >> "$tmp_manifest"
    printf '      "updater": {\n' >> "$tmp_manifest"
    printf '        "path": "%s",\n' "$manifest_updater_path" >> "$tmp_manifest"
    printf '        "sha256": "%s"\n' "$updater_sha" >> "$tmp_manifest"
    printf '      }\n' >> "$tmp_manifest"
  else
    printf '\n' >> "$tmp_manifest"
  fi

  printf '    }' >> "$tmp_manifest"
done

printf '\n  ]\n' >> "$tmp_manifest"
printf '}\n' >> "$tmp_manifest"
mv "$tmp_manifest" "$manifest"

echo "release manifest: $manifest"
