#!/bin/sh
# Installs the servef CLI built from the edge branch (latest main with the
# built frontend) on this machine. For development; stable installs use
# scripts/install.sh. Decision 0073.
#
#   curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-edge.sh | sh
#
# Needs git and Go. Writes only the servef binary to ~/.local/bin (or ~/bin
# when that is on PATH). Remove with scripts/uninstall.sh.

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

# shared:edge:begin
EDGE_URL="https://github.com/$REPO.git"

# clone_edge sets SRC to a shallow clone of the edge branch in TMP.
clone_edge() {
  has git || fail "git is required"
  SRC="$TMP/servef"
  step "cloning the edge branch of $REPO"
  git clone --quiet --depth 1 --branch edge "$EDGE_URL" "$SRC"
  ok "edge $(git -C "$SRC" rev-parse --short=12 HEAD): $(git -C "$SRC" log -1 --format=%s)"
}
# shared:edge:end

detect_platform
has go || fail "Go is required to build servef: install it from https://go.dev/dl/ (or run: mise use -g go)"
make_tmp
clone_edge

commit="$(git -C "$SRC" rev-parse --short=12 HEAD)"
date="$(git -C "$SRC" show -s --format=%cI HEAD)"
module="github.com/$REPO/internal/version"
step "building servef ($OS/$ARCH)"
(cd "$SRC" && CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X $module.Version=0.0.0-dev -X $module.Commit=$commit -X $module.Date=$date" \
  -o "$TMP/servef-bin" .)
install_cli "$TMP/servef-bin"
