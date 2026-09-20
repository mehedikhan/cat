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

say()  { printf '%s\n' "$*"; }
die()  { printf '%s\n' "error: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"; }

need uname
need tar
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1"; }
  fetch_to() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO- "$1"; }
  fetch_to() { wget -qO "$2" "$1"; }
else
  die "curl or wget is required"
fi

# --- platform -------------------------------------------------------------

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  *) die "unsupported operating system: $os (build from source instead)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch (build from source instead)" ;;
esac

# --- version --------------------------------------------------------------

version="${CATC_VERSION:-}"
if [ -z "$version" ]; then
  version=$(fetch "https://api.github.com/repos/$REPO/releases/latest" |
            sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
  [ -n "$version" ] || die "could not determine the latest release of $REPO"
fi

name="${BINARY}_${version}_${os}_${arch}"
url="https://github.com/$REPO/releases/download/$version/$name.tar.gz"

# --- destination ----------------------------------------------------------

if [ -n "${PREFIX:-}" ]; then
  bindir="$PREFIX/bin"
elif [ -w /usr/local/bin ] 2>/dev/null; then
  bindir=/usr/local/bin
elif [ "$(id -u)" = 0 ]; then
  bindir=/usr/local/bin
else
  bindir="$HOME/.local/bin"
fi

# --- install --------------------------------------------------------------

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "downloading $name"
fetch_to "$url" "$tmp/$name.tar.gz" || die "download failed: $url"

if command -v shasum >/dev/null 2>&1 || command -v sha256sum >/dev/null 2>&1; then
  if fetch_to "https://github.com/$REPO/releases/download/$version/checksums.txt" \
       "$tmp/checksums.txt" 2>/dev/null; then
    expected=$(grep " $name.tar.gz\$" "$tmp/checksums.txt" | awk '{print $1}')
    if [ -n "$expected" ]; then
      if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$tmp/$name.tar.gz" | awk '{print $1}')
      else
        actual=$(shasum -a 256 "$tmp/$name.tar.gz" | awk '{print $1}')
      fi
      [ "$expected" = "$actual" ] || die "checksum mismatch for $name.tar.gz"
      say "checksum verified"
    fi
  fi
fi

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
mkdir -p "$bindir"
install -m 0755 "$tmp/$name/$BINARY" "$bindir/$BINARY" 2>/dev/null || {
  cp "$tmp/$name/$BINARY" "$bindir/$BINARY" && chmod 0755 "$bindir/$BINARY"
} || die "could not write to $bindir (try: sudo sh install.sh, or PREFIX=\$HOME/.local)"

say "installed $bindir/$BINARY ($version)"

case ":$PATH:" in
  *":$bindir:"*) ;;
  *) say ""; say "note: $bindir is not on your PATH. Add it with:"
     say "  echo 'export PATH=\"$bindir:\$PATH\"' >> ~/.profile" ;;
esac

if ! command -v go >/dev/null 2>&1; then
  say ""
  say "warning: the Go toolchain was not found on your PATH."
  say "catc generates Go and builds it, so Go is required to compile programs."
  say "Install it from https://go.dev/dl/, then run: $BINARY doctor"
fi
