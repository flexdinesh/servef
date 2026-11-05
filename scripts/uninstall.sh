#!/bin/sh
# Removes servef from this machine: the CLI the user can write in ~/.local/bin,
# ~/bin, or Go's install directory; the desktop app, its desktop entry and icon;
# what the Electron app (decision 0095) and the servef-mygo app left. Prints how
# to remove package-managed copies (deb, Homebrew, mise). Never deletes user data
# (app data, page state, roots.json, caches). Decision 0073.
#
#   curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/uninstall.sh | sh

# The shared blocks define REPO and ARCH, which uninstalling never needs.
# shellcheck disable=SC2034

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
# beside it in servef-desktop/, which updates and uninstall leave alone.
DESKTOP_HOME="$DESKTOP_DATA/servef-desktop/app"
DESKTOP_ID="servef-desktop.desktop"
DESKTOP_ICON="$DESKTOP_DATA/icons/hicolor/512x512/apps/servef-desktop.png"
# The Electron app (decision 0095) installed its deb here, as servef-desktop or
# servef-electron, and on macOS as servef.app.
ELECTRON_DEB_DIR="${SERVEF_ELECTRON_DEB_DIR:-/opt/servef}"

desktop_running() {
  has pgrep || return 1
  if [ "$OS" = darwin ]; then
    pgrep -f 'servef-desktop\.app/Contents/MacOS/servef-desktop|servef\.app/Contents/MacOS/servef' >/dev/null 2>&1
  else
    pgrep -x -u "$(id -u)" 'servef-(desktop|electron)' >/dev/null 2>&1
  fi
}

# electron_paths CMD runs CMD with what the Electron app's scripts and its Open
# at Login left on Linux. Never its user data; with XDG_DATA_HOME equal to
# XDG_CONFIG_HOME, that shares servef-desktop/ with the AppImage.
electron_paths() {
  "$1" "$DESKTOP_DATA/servef-electron" \
    "$DESKTOP_DATA/servef-desktop/servef-desktop.AppImage" \
    "$DESKTOP_DATA/applications/servef-electron.desktop" \
    "$DESKTOP_DATA/icons/hicolor/512x512/apps/servef-electron.png" \
    "$DESKTOP_DATA/kio/servicemenus/servef-electron.desktop" \
    "$DESKTOP_DATA/kio/servicemenus/servef-desktop.desktop" \
    "$DESKTOP_DATA/nemo/actions/servef-electron.nemo_action" \
    "$DESKTOP_DATA/nemo/actions/servef-desktop.nemo_action" \
    "$DESKTOP_DATA/nautilus/scripts/Open in servef" \
    "$DESKTOP_CONFIG/autostart/servef-electron.desktop" \
    "$DESKTOP_CONFIG/autostart/servef-desktop.desktop"
}

# warn_thunar_action reports a hand-added Thunar action, which lives in Thunar's
# own settings file.
warn_thunar_action() {
  if [ -f "$DESKTOP_CONFIG/Thunar/uca.xml" ] && grep -Eq 'servef-(electron|desktop)' "$DESKTOP_CONFIG/Thunar/uca.xml"; then
    warn "Thunar still has an Open in servef custom action; remove it in Edit > Configure custom actions"
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
# when it is a servef-electron or servef-desktop AppImage (a manual install of
# the Electron app).
remove_appimage_of() {
  [ -f "$1" ] || return 0
  program="$(sed -n 's/^Exec=//p' "$1" | head -n 1 | sed -e 's/^"\([^"]*\)".*/\1/' -e t -e 's/ .*//')"
  case "$(basename "${program:-none}")" in
    servef-electron*.AppImage | servef-desktop*.AppImage) remove "$program" ;;
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
  fail "the servef desktop app is running; quit it, then run this again"
fi
# servef-mygo: the desktop app's name on edge before decision 0095.
mygo_running() {
  has pgrep || return 1
  if [ "$OS" = darwin ]; then
    pgrep -f 'servef-mygo\.app/Contents/MacOS/servef-mygo' >/dev/null 2>&1
  else
    pgrep -x -u "$(id -u)" servef-mygo >/dev/null 2>&1
  fi
}
if mygo_running; then
  fail "the servef-mygo app is running; quit it, then run this again"
fi
# The desktop app bundles a servef; the CLI may not be installed.
for program in "$(command -v servef 2>/dev/null || true)" "$DESKTOP_HOME/bin/servef" \
  "/Applications/servef-desktop.app/Contents/Resources/bin/servef" \
  "$HOME/Applications/servef-desktop.app/Contents/Resources/bin/servef"; do
  if [ -x "$program" ] && "$program" daemon status >/dev/null 2>&1; then
    fail "a servef daemon is running; stop it with '$program daemon stop', then run this again"
  fi
done

step "removing servef"
remove "$HOME/.local/bin/servef" "$HOME/bin/servef" "$(go_bin)/servef" \
  "$HOME/.local/bin/servef-desktop" "$HOME/bin/servef-desktop" \
  "$HOME/.local/bin/servef-mygo" "$HOME/bin/servef-mygo"

if [ "$OS" = linux ]; then
  for entry in "$DESKTOP_DATA/applications/servef-electron.desktop" "$DESKTOP_DATA/applications/$DESKTOP_ID" \
    "$DESKTOP_CONFIG/autostart/servef-electron.desktop" "$DESKTOP_CONFIG/autostart/$DESKTOP_ID"; do
    remove_appimage_of "$entry"
  done
  remove "$DESKTOP_HOME" "$DESKTOP_DATA/applications/$DESKTOP_ID" "$DESKTOP_ICON"
  electron_paths remove
  # Only what the scripts put there: WebKit's page state and, with XDG_DATA_HOME
  # equal to XDG_CONFIG_HOME, the Electron app's user data share this directory.
  rmdir "$DESKTOP_DATA/servef-desktop" 2>/dev/null || true
  remove "$DESKTOP_DATA/servef-mygo/app" "$DESKTOP_DATA/applications/servef-mygo.desktop" "$DESKTOP_DATA/icons/hicolor/512x512/apps/servef-mygo.png"
  rmdir "$DESKTOP_DATA/servef-mygo" 2>/dev/null || true
  if has update-desktop-database; then
    update-desktop-database "$DESKTOP_DATA/applications" >/dev/null 2>&1 || true
  fi
  warn_thunar_action
  for deb in servef-desktop servef-electron; do
    if [ -e "$ELECTRON_DEB_DIR/$deb" ]; then
      warn "the $deb deb is installed; remove it with:"
      note "sudo apt remove $deb"
    fi
  done
else
  for dir in /Applications "$HOME/Applications"; do
    for app in servef-desktop.app servef.app servef-mygo.app; do
      if [ -e "$dir/$app" ] && ! [ -w "$dir" ]; then
        warn "cannot remove $dir/$app; remove it with:"
        note "sudo rm -rf \"$dir/$app\""
      else
        remove "$dir/$app"
      fi
    done
  done
  remove "$HOME/Library/Services/Open in servef.workflow"
  note "If servef is in System Settings > General > Login Items, remove it there."
fi

# Copies on PATH that a package manager owns, or that this script does not know.
set -f
old_ifs="$IFS"
IFS=:
for dir in ${PATH:-}; do
  if [ -z "$dir" ] || ! [ -x "$dir/servef" ]; then continue; fi
  bin="$dir/servef"
  resolved="$bin"
  if [ -L "$bin" ]; then resolved="$(readlink "$bin" || true)"; fi
  case "$bin $resolved" in
    *Cellar* | *homebrew* | *linuxbrew*)
      warn "$bin is installed by Homebrew; remove it with:"
      note "brew uninstall flexdinesh/tap/servef"
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
  ok "uninstalled servef"
fi

if [ "$OS" = linux ]; then
  cache="${XDG_CACHE_HOME:-}"
  case "$cache" in
    /*) ;;
    *) cache="$HOME/.cache" ;;
  esac
  report_kept "$DESKTOP_DATA/servef-desktop" "$DESKTOP_CONFIG/servef-desktop" "$cache/servef-desktop" "$DESKTOP_CONFIG/servef" "$cache/servef" \
    "$DESKTOP_DATA/servef-mygo" "$DESKTOP_CONFIG/servef-mygo" "$cache/servef-mygo"
else
  support="$HOME/Library/Application Support"
  report_kept "$HOME/Library/WebKit/io.github.flexdinesh.servef.desktop" "$support/servef-desktop" "$support/servef" "$HOME/Library/Caches/servef" \
    "$HOME/Library/WebKit/io.github.flexdinesh.servef.mygo" "$support/servef-mygo"
fi
