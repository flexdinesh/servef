#!/bin/sh
# Removes srcz from this machine: the CLI the user can write in ~/.local/bin,
# ~/bin, or Go's install directory; the desktop app, its desktop entry and icon;
# what the Electron app (decision 0095) and the srcz-mygo app left. Prints how
# to remove package-managed copies (deb, Homebrew, mise). Never deletes user data
# (app data, page state, roots.json, caches). Decision 0073.
#
#   curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/uninstall.sh | sh

# The shared blocks define REPO and ARCH, which uninstalling never needs.
# shellcheck disable=SC2034

# shared:base:begin
set -eu

REPO="flexdinesh/srcz"

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
    *) fail "unsupported OS $(uname -s): srcz supports macOS and Linux" ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) ARCH=amd64 ;;
    aarch64 | arm64) ARCH=arm64 ;;
    *) fail "unsupported architecture $(uname -m): srcz supports amd64 and arm64" ;;
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
  TMP="$(mktemp -d 2>/dev/null || mktemp -d -t srcz)"
  trap cleanup EXIT
  trap 'exit 130' INT TERM
}
# shared:base:end

# shared:desktop:begin
DESKTOP_DATA="${XDG_DATA_HOME:-}"
case "$DESKTOP_DATA" in
  /*) ;;
  *) DESKTOP_DATA="$HOME/.local/share" ;;
esac
DESKTOP_CONFIG="${XDG_CONFIG_HOME:-}"
case "$DESKTOP_CONFIG" in
  /*) ;;
  *) DESKTOP_CONFIG="$HOME/.config" ;;
esac
# The app has a directory of its own: WebKit keeps the page's localStorage
# beside it in srcz-desktop/, which updates and uninstall leave alone.
DESKTOP_HOME="$DESKTOP_DATA/srcz-desktop/app"
DESKTOP_ID="srcz-desktop.desktop"
DESKTOP_ICON="$DESKTOP_DATA/icons/hicolor/512x512/apps/srcz-desktop.png"
# The Electron app (decision 0095) installed its deb here, as srcz-desktop or
# srcz-electron, and on macOS as srcz.app.
ELECTRON_DEB_DIR="${SRCZ_ELECTRON_DEB_DIR:-/opt/srcz}"

desktop_running() {
  has pgrep || return 1
  if [ "$OS" = darwin ]; then
    pgrep -f 'srcz-desktop\.app/Contents/MacOS/srcz-desktop|srcz\.app/Contents/MacOS/srcz' >/dev/null 2>&1
  else
    pgrep -x -u "$(id -u)" 'srcz-(desktop|electron)' >/dev/null 2>&1
  fi
}

# electron_paths CMD runs CMD with what the Electron app's scripts and its Open
# at Login left on Linux. Never its user data; with XDG_DATA_HOME equal to
# XDG_CONFIG_HOME, that shares srcz-desktop/ with the AppImage.
electron_paths() {
  "$1" "$DESKTOP_DATA/srcz-electron" \
    "$DESKTOP_DATA/srcz-desktop/srcz-desktop.AppImage" \
    "$DESKTOP_DATA/applications/srcz-electron.desktop" \
    "$DESKTOP_DATA/icons/hicolor/512x512/apps/srcz-electron.png" \
    "$DESKTOP_DATA/kio/servicemenus/srcz-electron.desktop" \
    "$DESKTOP_DATA/kio/servicemenus/srcz-desktop.desktop" \
    "$DESKTOP_DATA/nemo/actions/srcz-electron.nemo_action" \
    "$DESKTOP_DATA/nemo/actions/srcz-desktop.nemo_action" \
    "$DESKTOP_DATA/nautilus/scripts/Open in srcz" \
    "$DESKTOP_CONFIG/autostart/srcz-electron.desktop" \
    "$DESKTOP_CONFIG/autostart/srcz-desktop.desktop"
}

# warn_thunar_action reports a hand-added Thunar action, which lives in Thunar's
# own settings file.
warn_thunar_action() {
  if [ -f "$DESKTOP_CONFIG/Thunar/uca.xml" ] && grep -Eq 'srcz-(electron|desktop)' "$DESKTOP_CONFIG/Thunar/uca.xml"; then
    warn "Thunar still has an Open in srcz custom action; remove it in Edit > Configure custom actions"
  fi
}
# shared:desktop:end

REMOVED=0

# remove PATH... deletes each existing PATH.
remove() {
  for path in "$@"; do
    if [ -e "$path" ] || [ -L "$path" ]; then
      rm -rf "$path"
      ok "removed $path"
      REMOVED=$((REMOVED + 1))
    fi
  done
}

# remove_appimage_of ENTRY removes the AppImage a desktop entry's Exec runs,
# when it is a srcz-electron or srcz-desktop AppImage (a manual install of
# the Electron app).
remove_appimage_of() {
  [ -f "$1" ] || return 0
  program="$(sed -n 's/^Exec=//p' "$1" | head -n 1 | sed -e 's/^"\([^"]*\)".*/\1/' -e t -e 's/ .*//')"
  case "$(basename "${program:-none}")" in
    srcz-electron*.AppImage | srcz-desktop*.AppImage) remove "$program" ;;
  esac
}

# report_kept PATH... lists the user data directories that exist.
report_kept() {
  heading=1
  for path in "$@"; do
    [ -e "$path" ] || continue
    if [ "$heading" = 1 ]; then
      step "kept your data (delete it yourself if you no longer need it):"
      heading=0
    fi
    note "$path"
  done
}

# go_bin prints the directory go install writes to.
go_bin() {
  dir=""
  if has go; then
    dir="$(GOTOOLCHAIN=local go env GOBIN 2>/dev/null || true)"
    if [ -z "$dir" ]; then
      gopath="$(GOTOOLCHAIN=local go env GOPATH 2>/dev/null | cut -d: -f1 || true)"
      if [ -n "$gopath" ]; then dir="$gopath/bin"; fi
    fi
  fi
  printf '%s' "${dir:-$HOME/go/bin}"
}

detect_platform
if desktop_running; then
  fail "the srcz desktop app is running; quit it, then run this again"
fi
# srcz-mygo: the desktop app's name on edge before decision 0095.
mygo_running() {
  has pgrep || return 1
  if [ "$OS" = darwin ]; then
    pgrep -f 'srcz-mygo\.app/Contents/MacOS/srcz-mygo' >/dev/null 2>&1
  else
    pgrep -x -u "$(id -u)" srcz-mygo >/dev/null 2>&1
  fi
}
if mygo_running; then
  fail "the srcz-mygo app is running; quit it, then run this again"
fi
# The desktop app bundles a srcz; the CLI may not be installed.
for program in "$(command -v srcz 2>/dev/null || true)" "$DESKTOP_HOME/bin/srcz" \
  "/Applications/srcz-desktop.app/Contents/Resources/bin/srcz" \
  "$HOME/Applications/srcz-desktop.app/Contents/Resources/bin/srcz"; do
  if [ -x "$program" ] && "$program" daemon status >/dev/null 2>&1; then
    fail "a srcz daemon is running; stop it with '$program daemon stop', then run this again"
  fi
done

step "removing srcz"
remove "$HOME/.local/bin/srcz" "$HOME/bin/srcz" "$(go_bin)/srcz" \
  "$HOME/.local/bin/srcz-desktop" "$HOME/bin/srcz-desktop" \
  "$HOME/.local/bin/srcz-mygo" "$HOME/bin/srcz-mygo"

if [ "$OS" = linux ]; then
  for entry in "$DESKTOP_DATA/applications/srcz-electron.desktop" "$DESKTOP_DATA/applications/$DESKTOP_ID" \
    "$DESKTOP_CONFIG/autostart/srcz-electron.desktop" "$DESKTOP_CONFIG/autostart/$DESKTOP_ID"; do
    remove_appimage_of "$entry"
  done
  remove "$DESKTOP_HOME" "$DESKTOP_DATA/applications/$DESKTOP_ID" "$DESKTOP_ICON"
  electron_paths remove
  # Only what the scripts put there: WebKit's page state and, with XDG_DATA_HOME
  # equal to XDG_CONFIG_HOME, the Electron app's user data share this directory.
  rmdir "$DESKTOP_DATA/srcz-desktop" 2>/dev/null || true
  remove "$DESKTOP_DATA/srcz-mygo/app" "$DESKTOP_DATA/applications/srcz-mygo.desktop" "$DESKTOP_DATA/icons/hicolor/512x512/apps/srcz-mygo.png"
  rmdir "$DESKTOP_DATA/srcz-mygo" 2>/dev/null || true
  if has update-desktop-database; then
    update-desktop-database "$DESKTOP_DATA/applications" >/dev/null 2>&1 || true
  fi
  warn_thunar_action
  for deb in srcz-desktop srcz-electron; do
    if [ -e "$ELECTRON_DEB_DIR/$deb" ]; then
      warn "the $deb deb is installed; remove it with:"
      note "sudo apt remove $deb"
    fi
  done
else
  for dir in /Applications "$HOME/Applications"; do
    for app in srcz-desktop.app srcz.app srcz-mygo.app; do
      if [ -e "$dir/$app" ] && ! [ -w "$dir" ]; then
        warn "cannot remove $dir/$app; remove it with:"
        note "sudo rm -rf \"$dir/$app\""
      else
        remove "$dir/$app"
      fi
    done
  done
  remove "$HOME/Library/Services/Open in srcz.workflow"
  note "If srcz is in System Settings > General > Login Items, remove it there."
fi

# Copies on PATH that a package manager owns, or that this script does not know.
set -f
old_ifs="$IFS"
IFS=:
for dir in ${PATH:-}; do
  if [ -z "$dir" ] || ! [ -x "$dir/srcz" ]; then continue; fi
  bin="$dir/srcz"
  resolved="$bin"
  if [ -L "$bin" ]; then resolved="$(readlink "$bin" || true)"; fi
  case "$bin $resolved" in
    *Cellar* | *homebrew* | *linuxbrew*)
      warn "$bin is installed by Homebrew; remove it with:"
      note "brew uninstall flexdinesh/tap/srcz"
      ;;
    *mise*)
      warn "$bin is installed by mise; remove it with mise (mise uninstall)"
      ;;
    *)
      warn "$bin remains; remove it with the tool that installed it"
      ;;
  esac
done
IFS="$old_ifs"
set +f

if [ "$REMOVED" -eq 0 ]; then
  ok "nothing to remove in $HOME"
else
  ok "uninstalled srcz"
fi

if [ "$OS" = linux ]; then
  cache="${XDG_CACHE_HOME:-}"
  case "$cache" in
    /*) ;;
    *) cache="$HOME/.cache" ;;
  esac
  report_kept "$DESKTOP_DATA/srcz-desktop" "$DESKTOP_CONFIG/srcz-desktop" "$cache/srcz-desktop" "$DESKTOP_CONFIG/srcz" "$cache/srcz" \
    "$DESKTOP_DATA/srcz-mygo" "$DESKTOP_CONFIG/srcz-mygo" "$cache/srcz-mygo"
else
  support="$HOME/Library/Application Support"
  report_kept "$HOME/Library/WebKit/io.github.flexdinesh.srcz.desktop" "$support/srcz-desktop" "$support/srcz" "$HOME/Library/Caches/srcz" \
    "$HOME/Library/WebKit/io.github.flexdinesh.srcz.mygo" "$support/srcz-mygo"
fi
