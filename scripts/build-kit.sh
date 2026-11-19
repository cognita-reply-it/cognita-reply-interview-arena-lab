#!/bin/bash
# Cross-compile on Apple Silicon using the same pinned Go toolchain as build-macos.sh.
set -euo pipefail
umask 022
export COPYFILE_DISABLE=1
unset ZIPOPT UNZIP UNZIPOPT

usage() {
  printf '%s\n' 'Usage: scripts/build-kit.sh <linux-x64 | windows-x64> [--build-only | --stage-only] [--offline]' \
    'Cross-compiles the participant kit on Apple Silicon. Requires Python 3.' \
    '--build-only creates the application binary in a new .cache directory.' \
    '--stage-only assembles a complete kit there without creating a ZIP.' \
    '--offline uses only previously cached Go and llama.cpp archives.' \
    'By default, the complete kit ZIP and its SHA-256 file are saved under packages/.'
}
fail() { printf 'Build stopped: %s\n' "$*" >&2; exit 1; }

target="${1:-}"
case "$target" in
  linux-x64)
    target_os=linux
    package_target=Ubuntu-x64
    launcher='Start AI Interview Arena.sh'
    app_binary=ai-interview-arena
    engine_binary=llama-server
    ;;
  windows-x64)
    target_os=windows
    package_target=Windows-x64
    launcher='Start AI Interview Arena.cmd'
    app_binary=ai-interview-arena.exe
    engine_binary=llama-server.exe
    ;;
  -h|--help) usage; exit 0 ;;
  *) usage >&2; exit 2 ;;
esac
shift

mode=package
offline=0
for argument in "$@"; do
  case "$argument" in
    --build-only|--stage-only)
      [ "$mode" = package ] || fail 'Choose one build mode.'
      mode="$argument"
      ;;
    --offline) offline=1 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "Unknown option: $argument" ;;
  esac
done

[ "$(uname -s)" = Darwin ] && [ "$(uname -m)" = arm64 ] || fail 'Use an Apple Silicon Mac as the build host.'
command -v python3 >/dev/null || fail 'Python 3 is required for the build.'
kit_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$kit_root"

manifest_value() { /usr/bin/plutil -extract "$1" raw -o - "$2"; }
go_version="$(manifest_value build.go manifest.json)"
go_sha="$(manifest_value build.sha256 manifest.json)"
base_kit_version="$(manifest_value kit_version manifest.json)"
engine_version="$(manifest_value engine.version manifest.json)"
runtime_pins=scripts/desktop-runtimes.json
kit_version="$(manifest_value "$target.kit_version" "$runtime_pins")"
engine_url="$(manifest_value "$target.url" "$runtime_pins")"
engine_sha="$(manifest_value "$target.sha256" "$runtime_pins")"
[[ "$base_kit_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'Invalid kit version in manifest.json.'
[[ "$kit_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "Invalid kit version for $target in $runtime_pins."
[[ "$go_version" =~ ^go[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'Invalid Go version in manifest.json.'
[ "$engine_version" = b11233 ] && [ "$engine_version" = "$(manifest_value engine_version "$runtime_pins")" ] \
  || fail 'Update the Go runtime paths and the desktop runtime pins together.'

mkdir -p .cache .tools
staging="$(/usr/bin/mktemp -d "$kit_root/.cache/build-$target-XXXXXXXX")"
bundle_name="AI-Interview-Arena-$package_target-v$kit_version"
bundle="$staging/$bundle_name"
mkdir "$bundle"

# Stamp the platform version in the embedded HTML without writing to the shared frontend build.
python3 - "$staging" "$base_kit_version" "$kit_version" <<'PY'
import json
import sys
from pathlib import Path

staging, base_version, kit_version = sys.argv[1:]
staging = Path(staging)
source = Path("web/index.html").resolve()
html = source.read_bytes()
base_tag = f'<meta name="arena-kit-version" content="{base_version}">'.encode()
kit_tag = f'<meta name="arena-kit-version" content="{kit_version}">'.encode()
if html.count(base_tag) != 1:
    sys.exit("The embedded UI version differs from manifest.json. Run npm run build in frontend/.")
embedded_html = staging / "index.html"
embedded_html.write_bytes(html.replace(base_tag, kit_tag))
(staging / "overlay.json").write_text(
    json.dumps({"Replace": {str(source): str(embedded_html)}}), encoding="utf-8"
)
PY

check_sha() {
  local filename="$1" expected="$2" actual
  actual="$(/usr/bin/shasum -a 256 "$filename" | /usr/bin/awk '{print $1}')"
  [ "$actual" = "$expected" ] || fail "SHA-256 mismatch for $(basename "$filename"). File retained."
}
ensure_archive() {
  local url="$1" archive="$2" expected="$3" partial
  if [ ! -f "$archive" ]; then
    [ "$offline" -eq 0 ] || fail "Missing archive for offline build: $(basename "$archive")"
    partial="$staging/${archive##*/}.part"
    printf 'Downloading pinned dependency: %s\n' "$url"
    /usr/bin/curl --fail --location --proto '=https' --proto-redir '=https' --tlsv1.2 \
      --retry 3 --progress-bar --output "$partial" "$url"
    check_sha "$partial" "$expected"
    # Never truncate another build's verified archive.
    ln "$partial" "$archive" || fail 'Archive appeared during download. Both files are retained.'
  fi
  check_sha "$archive" "$expected"
  printf 'Verified archive: %s\n' "$(basename "$archive")"
}

go_archive=".cache/$go_version.darwin-arm64.tar.gz"
ensure_archive "https://go.dev/dl/$go_version.darwin-arm64.tar.gz" "$go_archive" "$go_sha"
if [ ! -d .tools/go ]; then
  /usr/bin/tar -xzf "$go_archive" -C "$staging"
fi
go_root="$kit_root/.tools/go"
if [ -d "$staging/go" ]; then
  go_root="$staging/go"
fi
go_bin="$go_root/bin/go"
[ -x "$go_bin" ] || fail 'The local Go toolchain is incomplete.'
export GOROOT="$go_root" GOCACHE="$kit_root/.cache/go-build" GOPATH="$kit_root/.cache/gopath"
export GOENV=off GOFLAGS= GOWORK=off GOEXPERIMENT= GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
actual_go="$("$go_bin" version)"
[ "$actual_go" = "go version $go_version darwin/arm64" ] || fail "Expected $go_version for darwin/arm64."
CGO_ENABLED=0 GOOS="$target_os" GOARCH=amd64 GOAMD64=v1 \
  "$go_bin" build -trimpath -buildvcs=false -overlay "$staging/overlay.json" -o "$bundle/$app_binary" .
chmod 755 "$bundle/$app_binary"
file "$bundle/$app_binary"
if [ "$mode" = --build-only ]; then
  printf 'Application binary: %s\n' "$bundle/$app_binary"
  exit 0
fi

# Include only reviewed materials. Models, logs, transcripts and local test evidence stay outside the kit.
for source_file in README.md LICENSE AI_Arena_Cognita_Reply_v4.pptx "$launcher" \
  configs/base/coach.md configs/base/scenario.json configs/base/cases.json configs/demo/coach.md configs/demo/scenario.json \
  docs/MODEL.md docs/VALIDATION.md docs/LAB.md \
  third_party/GO-LICENSE.txt \
  third_party/LLAMA-LICENSE.txt \
  third_party/QWEN-LICENSE.txt \
  third_party/QWEN-MODEL-CARD.md \
  third_party/QWEN3-BASE-MODEL-CARD.md \
  third_party/QWEN3-GGUF-MODEL-CARD.md \
  third_party/QWEN3-LICENSE.txt \
  third_party/README.md \
  third_party/UNSLOTH-MODEL-CARD.md \
  third_party/go-vendor-crypto-LICENSE.txt \
  third_party/go-vendor-net-LICENSE.txt \
  third_party/go-vendor-sys-LICENSE.txt \
  third_party/go-vendor-text-LICENSE.txt \
  third_party/llama-boringssl-LICENSE.txt \
  third_party/llama-boringssl-fiat-AUTHORS.txt \
  third_party/llama-boringssl-fiat-LICENSE.txt \
  third_party/llama-cpp-httplib-LICENSE.txt \
  third_party/llama-jsonhpp-LICENSE.txt \
  third_party/llama-llamafile-sgemm-LICENSE.txt \
  third_party/llama-miniaudio-LICENSE.txt \
  third_party/llama-rotate-bits-LICENSE.txt \
  third_party/llama-sha1-NOTICE.txt \
  third_party/llama-sha256-LICENSE.txt \
  third_party/llama-sheredom-subprocess-LICENSE.txt \
  third_party/llama-stb-image-LICENSE.txt \
  third_party/llama-ui-NOTICES.txt \
  third_party/llama-ui-big-integer-LICENSE.txt \
  third_party/llama-ui-nerdamer-prime-LICENSE.txt \
  third_party/llama-ui-sources.json \
  third_party/llama-xxhash-LICENSE.txt \
  third_party/sources.json; do
  [ -f "$source_file" ] && [ ! -L "$source_file" ] || fail "Required public file missing or symlinked: $source_file"
  mkdir -p "$bundle/$(dirname "$source_file")"
  cp "$source_file" "$bundle/$source_file"
  chmod 644 "$bundle/$source_file"
done

# Use the shared model and record the selected platform version and runtime provenance.
python3 - "$bundle/manifest.json" "$kit_version" "$engine_url" "$engine_sha" <<'PY'
import json
import sys
from pathlib import Path

manifest = json.loads(Path("manifest.json").read_text(encoding="utf-8"))
destination, kit_version, engine_url, engine_sha = sys.argv[1:]
manifest["kit_version"] = kit_version
manifest["engine"].update(url=engine_url, sha256=engine_sha)
Path(destination).write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
PY

engine_archive=".cache/${engine_url##*/}"
ensure_archive "$engine_url" "$engine_archive" "$engine_sha"
runtime="$bundle/runtime/$target"
engine_dir="$runtime/llama-$engine_version"
mkdir -p "$runtime"
# The pinned archives include the native backend libraries and upstream notices.
case "$target" in
  linux-x64)
    /usr/bin/tar -xzf "$engine_archive" -C "$runtime"
    chmod 755 "$bundle/$launcher"
    ;;
  windows-x64)
    mkdir "$engine_dir"
    /usr/bin/unzip -q "$engine_archive" -d "$engine_dir"
    [ -f "$engine_dir/LICENSE-LLVM-OpenMP" ] || fail 'The upstream OpenMP notice is missing.'
    ;;
esac
[ -f "$engine_dir/$engine_binary" ] || fail 'The runtime archive is missing the expected llama-server.'
chmod 755 "$engine_dir/$engine_binary"

if [ "$mode" = --stage-only ]; then
  printf 'Staged kit: %s\n' "$bundle"
  exit 0
fi

mkdir -p packages
package_name="$bundle_name.zip"
if [ -e "packages/$package_name" ] || [ -e "packages/$package_name.sha256" ]; then
  package_name="$bundle_name-${staging##*-}.zip"
fi
(
  cd "$staging"
  export TZ=UTC LC_ALL=C
  /usr/bin/find "$bundle_name" -exec /usr/bin/touch -h -t 198001010000 {} +
  # Keep Linux library symlinks and executable bits. Omit extra host metadata.
  /usr/bin/find "$bundle_name" -print | /usr/bin/sort | /usr/bin/zip -q -y -X "$package_name" -@
  python3 "$kit_root/scripts/check-publication.py" --archive "$package_name"
  /usr/bin/shasum -a 256 "$package_name" > "$package_name.sha256"
)
# Link only fully verified outputs. Keep prior builds and all staging directories.
ln "$staging/$package_name" "packages/$package_name"
ln "$staging/$package_name.sha256" "packages/$package_name.sha256"
printf 'Package: %s\nChecksum: %s.sha256\n' "$kit_root/packages/$package_name" "$kit_root/packages/$package_name"
