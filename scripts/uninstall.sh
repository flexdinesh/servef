#!/bin/sh
# Removes servef from this machine: the CLI the user can write in ~/.local/bin,
# ~/bin, or Go's install directory; the desktop app, desktop entry, icon, file
# manager actions, and login entry the install scripts or the documented
# manual steps create; the experimental native app (decision 0082). Prints how to remove package-managed copies (deb,
# Homebrew, mise). Never deletes user data (app data, roots.json, caches).
# Decision 0073.
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
DATA_HOME="${XDG_DATA_HOME:-}"
case "$DATA_HOME" in
  /*) ;;
  *) DATA_HOME="$HOME/.local/share" ;;
esac
CONFIG_HOME="${XDG_CONFIG_HOME:-}"
case "$CONFIG_HOME" in
  /*) ;;
  *) CONFIG_HOME="$HOME/.config" ;;
esac
APP_HOME="$DATA_HOME/servef-electron"
DESKTOP_ID="servef-electron.desktop"
ICON_PATH="$DATA_HOME/icons/hicolor/512x512/apps/servef-electron.png"
DOLPHIN_ACTION="$DATA_HOME/kio/servicemenus/servef-electron.desktop"
NEMO_ACTION="$DATA_HOME/nemo/actions/servef-electron.nemo_action"
DEB_EXECUTABLE="/opt/servef/servef-electron"
# Installs from before the rename (decision 0092).
OLD_APP_HOME="$DATA_HOME/servef-desktop"
OLD_DESKTOP_ID="servef-desktop.desktop"
OLD_ICON_PATH="$DATA_HOME/icons/hicolor/512x512/apps/servef-desktop.png"
OLD_DOLPHIN_ACTION="$DATA_HOME/kio/servicemenus/servef-desktop.desktop"
OLD_NEMO_ACTION="$DATA_HOME/nemo/actions/servef-desktop.nemo_action"
OLD_DEB_EXECUTABLE="/opt/servef/servef-desktop"

app_running() {
  has pgrep || return 1
  if [ "$OS" = darwin ]; then
    pgrep -f 'servef\.app/Contents/MacOS/servef' >/dev/null 2>&1
  else
    pgrep -x -u "$(id -u)" 'servef-(desktop|electron)' >/dev/null 2>&1
  fi
}

check_desktop_conflicts() {
  if [ "$OS" = linux ] && [ -e "$DEB_EXECUTABLE" ]; then
    fail "the servef-electron deb is installed ($DEB_EXECUTABLE); keep using it, or remove it first: sudo apt remove servef-electron"
  fi
  if [ "$OS" = linux ] && [ -e "$OLD_DEB_EXECUTABLE" ]; then
    fail "the servef-desktop deb is installed ($OLD_DEB_EXECUTABLE); install the servef-electron deb over it, or remove it first: sudo apt remove servef-desktop"
  fi
  if app_running; then
    fail "the servef desktop app is running; quit it, then run this again"
  fi
}
# shared:desktop:end

# shared:native:begin
NATIVE_DATA="${XDG_DATA_HOME:-}"
case "$NATIVE_DATA" in
  /*) ;;
  *) NATIVE_DATA="$HOME/.local/share" ;;
esac
NATIVE_HOME="$NATIVE_DATA/servef-native"
NATIVE_DESKTOP_ID="servef-native.desktop"
NATIVE_ICON="$NATIVE_DATA/icons/hicolor/512x512/apps/servef-native.png"

native_running() {
  has pgrep || return 1
  if [ "$OS" = darwin ]; then
    pgrep -f 'servef-native\.app/Contents/MacOS/servef-native' >/dev/null 2>&1
  else
    pgrep -x -u "$(id -u)" servef-native >/dev/null 2>&1
  fi
}
# shared:native:end

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
# when it is a servef-electron or servef-desktop AppImage (a manual AppImage
# install).
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
if app_running; then
  fail "the servef desktop app is running; quit it, then run this again"
fi
if native_running; then
  fail "the servef native app is running; quit it, then run this again"
fi
if has servef && servef daemon status >/dev/null 2>&1; then
  fail "a servef daemon is running; stop it with 'servef daemon stop', then run this again"
fi

step "removing servef"
remove "$HOME/.local/bin/servef" "$HOME/bin/servef" "$(go_bin)/servef" \
  "$HOME/.local/bin/servef-native" "$HOME/bin/servef-native"

if [ "$OS" = linux ]; then
  entry="$DATA_HOME/applications/$DESKTOP_ID"
  autostart="$CONFIG_HOME/autostart/$DESKTOP_ID"
  old_entry="$DATA_HOME/applications/$OLD_DESKTOP_ID"
  old_autostart="$CONFIG_HOME/autostart/$OLD_DESKTOP_ID"
  remove_appimage_of "$entry"
  remove_appimage_of "$autostart"
  remove_appimage_of "$old_entry"
  remove_appimage_of "$old_autostart"
  remove "$APP_HOME" "$entry" "$autostart" "$ICON_PATH" "$DOLPHIN_ACTION" "$NEMO_ACTION" \
    "$OLD_APP_HOME" "$old_entry" "$old_autostart" "$OLD_ICON_PATH" "$OLD_DOLPHIN_ACTION" "$OLD_NEMO_ACTION" \
    "$DATA_HOME/nautilus/scripts/Open in servef"
  if has update-desktop-database; then
    update-desktop-database "$DATA_HOME/applications" >/dev/null 2>&1 || true
  fi
  if [ -f "$CONFIG_HOME/Thunar/uca.xml" ] && grep -Eq 'servef-(electron|desktop)' "$CONFIG_HOME/Thunar/uca.xml"; then
    warn "Thunar still has an Open in servef custom action; remove it in Edit > Configure custom actions"
  fi
  remove "$NATIVE_HOME" "$NATIVE_DATA/applications/$NATIVE_DESKTOP_ID" "$NATIVE_ICON"
  if [ -e "$DEB_EXECUTABLE" ]; then
    warn "the servef-electron deb is installed; remove it with:"
    note "sudo apt remove servef-electron"
  fi
  if [ -e "$OLD_DEB_EXECUTABLE" ]; then
    warn "the servef-desktop deb is installed; remove it with:"
    note "sudo apt remove servef-desktop"
  fi
else
  for dir in /Applications "$HOME/Applications"; do
    for app in servef.app servef-native.app; do
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
  report_kept "$CONFIG_HOME/servef-desktop" "$CONFIG_HOME/servef-native" "$CONFIG_HOME/servef" "$cache/servef"
else
  support="$HOME/Library/Application Support"
  report_kept "$support/servef-desktop" "$support/servef-native" "$support/servef" "$HOME/Library/Caches/servef"
fi
