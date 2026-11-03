#!/bin/sh
# Installs the servef CLI from the latest release. Decision 0073.
#
#   curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install.sh | sh
#
# Writes only the servef binary to ~/.local/bin (or ~/bin when that is on
# PATH). The desktop app and CLI: scripts/install-desktop.sh. Latest main:
# scripts/install-edge.sh. Remove with scripts/uninstall.sh.

# shared:base:begin
set -eu

REPO="flexdinesh/servef"

if [ -t 1 ]; then
  BOLD="$(printf '\033[1m')"
  GREEN="$(printf '\033[32m')"
  YELLOW="$(printf '\033[33m')"
  RED="$(printf '\033[31m')"
  RESET="$(printf '\033[0m')"
else
  BOLD=""
  GREEN=""
  YELLOW=""
  RED=""
  RESET=""
fi

step() { printf ' %s>%s %s\n' "$YELLOW" "$RESET" "$1"; }
ok() { printf ' %s✓%s %s\n' "$GREEN" "$RESET" "$1"; }
warn() { printf ' %s!%s %s\n' "$YELLOW" "$RESET" "$1"; }
fail() {
  printf ' %s✗ %s%s\n' "$RED" "$1" "$RESET" >&2
  exit 1
}
note() { printf '     %s%s%s\n' "$BOLD" "$1" "$RESET"; }
has() { command -v "$1" >/dev/null 2>&1; }

[ -n "${HOME:-}" ] || fail "HOME is not set"

detect_platform() {
  case "$(uname -s)" in
    Linux) OS=linux ;;
    Darwin) OS=darwin ;;
    *) fail "unsupported OS $(uname -s): servef supports macOS and Linux" ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) ARCH=amd64 ;;
    aarch64 | arm64) ARCH=arm64 ;;
    *) fail "unsupported architecture $(uname -m): servef supports amd64 and arm64" ;;
  esac
}

# fetch URL OUT downloads URL to OUT.
fetch() {
  if has curl; then
    curl -fsSL --retry 2 --connect-timeout 10 -o "$2" "$1"
  elif has wget; then
    wget -q -O "$2" "$1"
  else
    fail "curl or wget is required"
  fi
}

TMP=""
cleanup() {
  if [ -n "$TMP" ]; then rm -rf "$TMP"; fi
}
make_tmp() {
  TMP="$(mktemp -d 2>/dev/null || mktemp -d -t servef)"
  trap cleanup EXIT
  trap 'exit 130' INT TERM
}
# shared:base:end

# shared:cli:begin
on_path() {
  case ":${PATH:-}:" in
    *":$1:"*) return 0 ;;
  esac
  return 1
}

# choose_bin_dir sets BIN_DIR: a writable directory already on PATH
# (~/.local/bin, then ~/bin), else ~/.local/bin, created if needed.
choose_bin_dir() {
  for dir in "$HOME/.local/bin" "$HOME/bin"; do
    if on_path "$dir" && mkdir -p "$dir" 2>/dev/null && [ -w "$dir" ]; then
      BIN_DIR="$dir"
      return 0
    fi
  done
  BIN_DIR="$HOME/.local/bin"
  if ! mkdir -p "$BIN_DIR" 2>/dev/null || ! [ -w "$BIN_DIR" ]; then
    fail "cannot write $BIN_DIR"
  fi
}

# install_cli SRC installs the servef binary SRC as BIN_DIR/servef.
install_cli() {
  choose_bin_dir
  target="$BIN_DIR/servef"
  previous=""
  if [ -x "$target" ]; then
    previous="$("$target" --version 2>/dev/null | head -n 1 || true)"
  fi
  staged="$BIN_DIR/.servef.$$"
  cp "$1" "$staged"
  chmod 755 "$staged"
  mv -f "$staged" "$target"
  current="$("$target" --version 2>/dev/null | head -n 1 || true)"
  if [ -n "$previous" ]; then
    ok "updated $target ($previous -> $current)"
  else
    ok "installed $target ($current)"
  fi

  if ! on_path "$BIN_DIR"; then
    warn "$BIN_DIR is not on PATH; add it:"
    case "$(basename "${SHELL:-sh}")" in
      fish) note "fish_add_path \"$BIN_DIR\"" ;;
      zsh) note "export PATH=\"$BIN_DIR:\$PATH\"   # in ~/.zshrc" ;;
      bash) note "export PATH=\"$BIN_DIR:\$PATH\"   # in ~/.bashrc" ;;
      *) note "export PATH=\"$BIN_DIR:\$PATH\"   # in ~/.profile" ;;
    esac
  fi
  first="$(command -v servef 2>/dev/null || true)"
  if [ -n "$first" ] && [ "$first" != "$target" ]; then
    warn "$first comes first on PATH and is not the servef just installed"
  fi
  if "$target" daemon status >/dev/null 2>&1; then
    warn "a servef daemon is running an earlier build; to switch now, run:"
    note "servef daemon stop"
  fi
}
# shared:cli:end

# shared:latest:begin
# latest_release sets TAG and VERSION (TAG without its leading v) from the tag
# the releases/latest page redirects to; the GitHub API is rate limited.
latest_release() {
  url="https://github.com/$REPO/releases/latest"
  location=""
  if has curl; then
    location="$(curl -fsS -o /dev/null -w '%{redirect_url}' "$url" || true)"
  elif has wget; then
    location="$(wget -S --max-redirect=0 -O /dev/null "$url" 2>&1 | sed -n 's/^ *[Ll]ocation: *//p' | tr -d '\r' | tail -n 1)"
  else
    fail "curl or wget is required"
  fi
  tag="${location##*/tag/}"
  case "$tag" in
    v[0-9]*)
      TAG="$tag"
      VERSION="${tag#v}"
      ;;
    *) fail "could not find the latest servef release at $url" ;;
  esac
}
# shared:latest:end

# shared:release:begin
# release_url FILE prints FILE's download URL in release TAG.
release_url() {
  printf 'https://github.com/%s/releases/download/%s/%s' "$REPO" "$TAG" "$1"
}

# download_cli sets CLI to the servef binary of release TAG, whose assets are
# named for VERSION.
download_cli() {
  name="servef_${VERSION}_${OS}_${ARCH}"
  step "downloading servef $TAG ($OS/$ARCH)"
  fetch "$(release_url "$name.tar.gz")" "$TMP/$name.tar.gz" || fail "could not download $name.tar.gz from servef $TAG"
  tar -xzf "$TMP/$name.tar.gz" -C "$TMP"
  CLI="$TMP/$name/servef"
  [ -f "$CLI" ] || fail "$name.tar.gz has no servef binary"
}
# shared:release:end

detect_platform
make_tmp
latest_release
download_cli
install_cli "$CLI"
ok "done; open the current directory with:"
note "servef ."
