#!/bin/bash
# Developer build only. The packaged kit requires none of these development tools.
# Archives, staging folders and old packages are retained; this script never deletes them.
set -euo pipefail

kit_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
build_only=0
stage_only=0
offline=0
for arena_arg in "$@"; do
  case "$arena_arg" in
    --build-only) build_only=1 ;;
    --stage-only) stage_only=1 ;;
    --offline) offline=1 ;;
    -h|--help)
      printf '%s\n' 'Usage: scripts/build-macos.sh [--build-only | --stage-only] [--offline]' \
        'Builds with pinned Go and llama.cpp archives from manifest.json.' \
        'Missing archives download automatically unless --offline is given.' \
        '--build-only creates a native binary in a unique .cache build folder, without a ZIP.' \
        '--stage-only assembles the full participant kit in that folder, without a ZIP.' \
        'The default also creates a complete participant ZIP under packages/.'
      exit 0 ;;
    *) printf 'Unknown option: %s\n' "$arena_arg" >&2; exit 2 ;;
  esac
done
fail() { printf '\nBuild stopped: %s\n' "$*" >&2; exit 1; }
[ "$build_only" -eq 0 ] || [ "$stage_only" -eq 0 ] || fail 'Choose either --build-only or --stage-only, not both.'
[ "$(uname -s)" = Darwin ] && [ "$(uname -m)" = arm64 ] || fail 'Use an Apple Silicon Mac. Other platforms are not verified.'
macos_version="$(/usr/bin/sw_vers -productVersion)"
macos_major="${macos_version%%.*}"
macos_remaining="${macos_version#*.}"
macos_minor="${macos_remaining%%.*}"
if [ "$macos_major" -lt 13 ] || { [ "$macos_major" -eq 13 ] && [ "$macos_minor" -lt 3 ]; }; then
  fail 'The pinned llama.cpp runtime requires macOS 13.3 or newer.'
fi
cd "$kit_root"
manifest_value() { /usr/bin/plutil -extract "$1" raw -o - "$kit_root/manifest.json"; }
go_version="$(manifest_value build.go)"
go_sha="$(manifest_value build.sha256)"
engine_version="$(manifest_value engine.version)"
engine_url="$(manifest_value engine.url)"
engine_sha="$(manifest_value engine.sha256)"
kit_version="$(manifest_value kit_version)"
[[ "$kit_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'Invalid kit_version in manifest.json.'
/usr/bin/grep -Fqx "<meta name=\"arena-kit-version\" content=\"$kit_version\">" "$kit_root/web/index.html" \
  || fail 'The embedded UI version differs from manifest.json. Run npm run build in frontend/.'
# Runtime path is deliberately fixed by the source: changing dependencies is an explicit update.
[ "$engine_version" = b11233 ] || fail 'Update the runtime path in platform_darwin.go before changing engine.version.'
mkdir -p "$kit_root/.cache" "$kit_root/.tools"

check_sha() {
  local arena_file="$1" arena_expected="$2" arena_actual
  arena_actual="$(/usr/bin/shasum -a 256 "$arena_file" | /usr/bin/awk '{print $1}')"
  [ "$arena_actual" = "$arena_expected" ] || fail "SHA-256 mismatch: $arena_file. File retained; inspect it before retrying."
}
ensure_archive() {
  local arena_url="$1" arena_file="$2" arena_expected="$3"
  if [ -f "$arena_file" ]; then
    check_sha "$arena_file" "$arena_expected"
    printf 'Verified archive: %s\n' "$(basename "$arena_file")"
    return
  fi
  [ "$offline" -eq 0 ] || fail "Missing archive in offline build: $arena_file"
  printf 'Downloading pinned dependency: %s\n' "$arena_url"
  /usr/bin/curl --fail --location --proto '=https' --proto-redir '=https' --tlsv1.2 \
    --retry 3 --continue-at - --progress-bar --output "$arena_file.part" "$arena_url" \
    || fail "Download interrupted. Partial file retained at $arena_file.part; run this command again to resume."
  check_sha "$arena_file.part" "$arena_expected"
  [ ! -e "$arena_file" ] || fail "Archive appeared while downloading: $arena_file. Both files retained."
  mv "$arena_file.part" "$arena_file"
}

go_archive="$kit_root/.cache/$go_version.darwin-arm64.tar.gz"
engine_archive="$kit_root/.cache/llama-$engine_version-bin-macos-arm64.tar.gz"
ensure_archive "https://go.dev/dl/$go_version.darwin-arm64.tar.gz" "$go_archive" "$go_sha"
ensure_archive "$engine_url" "$engine_archive" "$engine_sha"
if [ ! -d "$kit_root/.tools/go" ]; then
  /usr/bin/tar -xzf "$go_archive" -C "$kit_root/.tools"
fi
[ -x "$kit_root/.tools/go/bin/go" ] || fail '.tools/go exists but is incomplete. Move it to Trash manually and retry.'
export GOROOT="$kit_root/.tools/go"
export GOCACHE="$kit_root/.cache/go-build"
export GOPATH="$kit_root/.cache/gopath"
export GOTOOLCHAIN=local
export GOPROXY=off
export GOSUMDB=off
actual_go="$("$GOROOT/bin/go" version)"
[ "$actual_go" = "go version $go_version darwin/arm64" ] || fail "Unexpected development toolchain: $actual_go. Expected $go_version."

build_stamp="$(date +%Y%m%d-%H%M%S)-$$"
staging="$kit_root/.cache/build-macos-$build_stamp"
[ ! -e "$staging" ] || fail "Build folder already exists: $staging"
bundle_name="AI-Interview-Arena-macOS-arm64-v$kit_version"
bundle="$staging/$bundle_name"
mkdir -p "$bundle"
printf 'Building native application with %s…\n' "$go_version"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 "$GOROOT/bin/go" build -trimpath -buildvcs=false -o "$bundle/ai-interview-arena" .
/usr/bin/file "$bundle/ai-interview-arena"
if [ "$build_only" -eq 1 ]; then
  printf '\nBuild complete; no package created.\nBinary: %s\n' "$bundle/ai-interview-arena"
  printf '%s\n' 'To run this binary, pass --root pointing to a complete kit with runtime, manifest and configs.'
  printf '%s\n' 'Use --stage-only to assemble the complete kit, or no mode flag to also create its ZIP.'
  exit 0
fi

# Explicit inclusion: no model weights, operational logs, transcripts, caches or development runtimes.
for required_file in README.md LICENSE manifest.json AI_Arena_Cognita_Reply_v4.pptx 'Start AI Interview Arena.command' docs/MODEL.md docs/VALIDATION.md docs/LAB.md configs/base/cases.json; do
  [ -f "$kit_root/$required_file" ] || fail "Required package file is missing: $required_file"
done
cp "$kit_root/README.md" "$kit_root/AI_Arena_Cognita_Reply_v4.pptx" "$kit_root/LICENSE" "$kit_root/manifest.json" "$kit_root/Start AI Interview Arena.command" "$bundle/"
# Copy only public configuration files; editor metadata and local experiments stay out.
for config_name in base demo; do
  mkdir -p "$bundle/configs/$config_name"
  cp "$kit_root/configs/$config_name/coach.md" "$kit_root/configs/$config_name/scenario.json" "$bundle/configs/$config_name/"
done
mkdir -p "$bundle/third_party"
# Upstream licenses and provenance are an explicit, reviewed allowlist.
for notice in third_party/GO-LICENSE.txt \
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
  cp "$kit_root/$notice" "$bundle/third_party/"
done
mkdir -p "$bundle/docs" "$bundle/runtime/macos-arm64"
# Explicit public documentation allowlist. Local reports and test harnesses never enter the kit.
cp "$kit_root/docs/MODEL.md" "$kit_root/docs/VALIDATION.md" "$kit_root/docs/LAB.md" "$bundle/docs/"
cp "$kit_root/configs/base/cases.json" "$bundle/configs/base/"
# Preserve every file and dylib/symlink shipped in the verified upstream runtime archive.
/usr/bin/tar -xzf "$engine_archive" -C "$bundle/runtime/macos-arm64"
chmod +x "$bundle/ai-interview-arena" "$bundle/Start AI Interview Arena.command"
[ -x "$bundle/runtime/macos-arm64/llama-$engine_version/llama-server" ] || fail 'The verified runtime archive does not contain the expected llama-server.'
if [ "$stage_only" -eq 1 ]; then
  printf '\nComplete staged kit: %s\nNo ZIP created. Models and operational logs are absent.\n' "$bundle"
  printf 'Launch for validation: "%s/Start AI Interview Arena.command"\n' "$bundle"
  exit 0
fi
mkdir -p "$kit_root/packages"
package_path="$kit_root/packages/$bundle_name.zip"
if [ -e "$package_path" ] || [ -e "$package_path.sha256" ]; then
  package_path="$kit_root/packages/$bundle_name-$build_stamp.zip"
fi
[ ! -e "$package_path" ] && [ ! -e "$package_path.sha256" ] || fail "Package path already exists: $package_path"
# -X omits host UID/GID and other local extra fields; -y preserves runtime symlinks.
(
  cd "$staging"
  /usr/bin/zip -q -r -y -X "$package_path" "$bundle_name"
)
(
  cd "$kit_root/packages"
  /usr/bin/shasum -a 256 "$(basename "$package_path")" > "$(basename "$package_path").sha256"
)
printf '\nPackage: %s\nChecksum: %s.sha256\nRetained staging folder: %s\n' "$package_path" "$package_path" "$staging"
printf '%s\n' 'No model weights are included. First launch downloads and verifies the pinned model.'
