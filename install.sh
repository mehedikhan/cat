#!/bin/sh
# Install catc, the cat compiler.
#
#   curl -fsSL https://raw.githubusercontent.com/mehedikhan/cat/main/install.sh | sh
#
# Environment:
#   CATC_VERSION   version to install (default: latest release)
#   PREFIX         install prefix (default: /usr/local, or ~/.local if that
#                  is not writable)

set -eu

REPO="mehedikhan/cat"
BINARY="catc"
API="https://api.github.com/repos/$REPO"

say() { printf '%s\n' "$*"; }
die() { printf '%s\n' "error: $*" >&2; exit 1; }

command -v uname >/dev/null 2>&1 || die "uname is required"
command -v tar   >/dev/null 2>&1 || die "tar is required"

# http_to URL FILE -> prints the HTTP status code
if command -v curl >/dev/null 2>&1; then
  http_to() { curl -sSL -o "$2" -w '%{http_code}' "$1" 2>/dev/null || echo 000; }
elif command -v wget >/dev/null 2>&1; then
  # wget cannot report the status portably; 200 or 000 is the best it can do.
  http_to() { if wget -q -O "$2" "$1" 2>/dev/null; then echo 200; else echo 000; fi; }
else
  die "curl or wget is required"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# --- platform -------------------------------------------------------------

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  *) die "unsupported operating system: $os (build from source instead)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch (build from source instead)" ;;
esac

# --- version --------------------------------------------------------------

from_source_hint() {
  if command -v go >/dev/null 2>&1; then
    say ""
    say "Since you have Go installed, you can build from source instead:"
    say "  go install github.com/$REPO/cmd/$BINARY@latest"
  else
    say ""
    say "To build from source, install Go from https://go.dev/dl/ and run:"
    say "  go install github.com/$REPO/cmd/$BINARY@latest"
  fi
}

version="${CATC_VERSION:-}"
if [ -z "$version" ]; then
  status=$(http_to "$API/releases/latest" "$tmp/release.json")
  case "$status" in
    200)
      version=$(sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' "$tmp/release.json" | head -1)
      [ -n "$version" ] || die "could not parse the latest release of $REPO"
      ;;
    404)
      # Distinguish "no such repo" from "repo exists but has no releases".
      repo_status=$(http_to "$API" "$tmp/repo.json")
      if [ "$repo_status" = "404" ]; then
        say "error: repository $REPO was not found." >&2
        say "" >&2
        say "It may not exist yet, or it may be private. Check the name at" >&2
        say "  https://github.com/$REPO" >&2
        from_source_hint >&2
      else
        say "error: $REPO has no published releases yet." >&2
        say "" >&2
        say "Prebuilt binaries are attached to GitHub releases; there are none" >&2
        say "to download. Maintainer: push a version tag to publish one." >&2
        from_source_hint >&2
      fi
      exit 1
      ;;
    403|429)
      die "GitHub rate-limited this request (HTTP $status). Wait a few minutes,
       or pick a version explicitly: CATC_VERSION=v0.1.0 sh install.sh"
      ;;
    000)
      die "could not reach api.github.com (network or proxy problem)"
      ;;
    *)
      die "GitHub API returned HTTP $status for $API/releases/latest"
      ;;
  esac
fi

name="${BINARY}_${version}_${os}_${arch}"
base="https://github.com/$REPO/releases/download/$version"

# --- destination ----------------------------------------------------------

if [ -n "${PREFIX:-}" ]; then
  bindir="$PREFIX/bin"
elif [ -w /usr/local/bin ] 2>/dev/null || [ "$(id -u)" = 0 ]; then
  bindir=/usr/local/bin
else
  bindir="$HOME/.local/bin"
fi

# --- download -------------------------------------------------------------

say "downloading $name"
status=$(http_to "$base/$name.tar.gz" "$tmp/$name.tar.gz")
if [ "$status" != "200" ]; then
  if [ "$status" = "404" ]; then
    say "error: release $version has no build for ${os}/${arch}." >&2
    say "       expected asset: $name.tar.gz" >&2
    say "       see https://github.com/$REPO/releases/tag/$version" >&2
  else
    say "error: download failed (HTTP $status): $base/$name.tar.gz" >&2
  fi
  exit 1
fi

# --- checksum -------------------------------------------------------------

if command -v sha256sum >/dev/null 2>&1; then
  sum() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sum() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  sum() { echo ""; }
fi

if [ "$(http_to "$base/checksums.txt" "$tmp/checksums.txt")" = "200" ]; then
  expected=$(grep " $name.tar.gz\$" "$tmp/checksums.txt" | awk '{print $1}')
  actual=$(sum "$tmp/$name.tar.gz")
  if [ -n "$expected" ] && [ -n "$actual" ]; then
    [ "$expected" = "$actual" ] || die "checksum mismatch for $name.tar.gz
       expected $expected
       got      $actual"
    say "checksum verified"
  fi
fi

# --- install --------------------------------------------------------------

tar -xzf "$tmp/$name.tar.gz" -C "$tmp" || die "could not extract $name.tar.gz"
[ -f "$tmp/$name/$BINARY" ] || die "archive did not contain $BINARY"

mkdir -p "$bindir" 2>/dev/null || die "could not create $bindir"
if ! cp "$tmp/$name/$BINARY" "$bindir/$BINARY" 2>/dev/null; then
  die "could not write to $bindir

       Try one of:
         sudo sh install.sh
         PREFIX=\$HOME/.local sh install.sh"
fi
chmod 0755 "$bindir/$BINARY"

say "installed $bindir/$BINARY ($version)"

case ":$PATH:" in
  *":$bindir:"*) ;;
  *)
    say ""
    say "note: $bindir is not on your PATH. Add it with:"
    say "  echo 'export PATH=\"$bindir:\$PATH\"' >> ~/.zshrc"
    ;;
esac

if ! command -v go >/dev/null 2>&1; then
  say ""
  say "warning: the Go toolchain was not found on your PATH."
  say "catc generates Go and builds it, so Go is required to compile programs."
  say "Install it from https://go.dev/dl/, then run: $BINARY doctor"
fi
