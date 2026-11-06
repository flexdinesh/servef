#!/bin/sh
# Installs the srcz desktop app (decision 0095) from the latest release. It
# bundles the srcz binary it starts the daemon with; the CLI is installed
# separately (scripts/install.sh).
#
#   curl -fsSL https://raw.githubusercontent.com/flexdinesh/srcz/main/scripts/install-desktop.sh | sh
#
# Linux: the app goes to ~/.local/share/srcz-desktop/app with a desktop entry
# for srcz:// links and an icon. macOS: srcz-desktop.app in /Applications
# (or ~/Applications). srcz-desktop goes on PATH in ~/.local/bin (or ~/bin
# when that is on PATH). Replaces an Electron install and keeps its data. Latest
# main: scripts/install-desktop-edge.sh. Remove with scripts/uninstall.sh.

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

# install_cli SRC installs the srcz binary SRC as BIN_DIR/srcz.
install_cli() {
  choose_bin_dir
  target="$BIN_DIR/srcz"
  previous=""
  if [ -x "$target" ]; then
    previous="$("$target" --version 2>/dev/null | head -n 1 || true)"
  fi
  staged="$BIN_DIR/.srcz.$$"
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
  first="$(command -v srcz 2>/dev/null || true)"
  if [ -n "$first" ] && [ "$first" != "$target" ]; then
    warn "$first comes first on PATH and is not the srcz just installed"
  fi
  if "$target" daemon status >/dev/null 2>&1; then
    warn "a srcz daemon is running an earlier build; to switch now, run:"
    note "srcz daemon stop"
  fi
}
# shared:cli:end

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

# shared:desktop-install:begin
# check_webkit warns on Linux without WebKitGTK 4.1, which the app's window
# needs. The app still installs: the check cannot see every library path.
check_webkit() {
  [ "$OS" = linux ] || return 0
  finder="$(command -v ldconfig 2>/dev/null || true)"
  if [ -z "$finder" ] && [ -x /sbin/ldconfig ]; then finder=/sbin/ldconfig; fi
  [ -n "$finder" ] || return 0
  if ! "$finder" -p 2>/dev/null | grep -q 'libwebkit2gtk-4\.1\.so\.0'; then
    warn "WebKitGTK 4.1 not found; the app needs it to open a window. Debian, Ubuntu:"
    note "sudo apt install libwebkit2gtk-4.1-0"
  fi
}

LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

check_desktop_conflicts() {
  if [ "$OS" = linux ]; then
    for deb in srcz-desktop srcz-electron; do
      if [ -e "$ELECTRON_DEB_DIR/$deb" ]; then
        fail "the $deb deb (the Electron app) is installed; remove it first: sudo apt remove $deb"
      fi
    done
  fi
  if desktop_running; then
    fail "the srcz desktop app is running; quit it, then run this again"
  fi
}

# desktop_exec PATH renders PATH as one desktop entry Exec argument.
desktop_exec() {
  value="$(printf '%s' "$1" | sed 's/%/%%/g')"
  case "$value" in
    *[!A-Za-z0-9_./+,:@=-]*)
      # shellcheck disable=SC2016 # sed expression, not shell expansion
      value="\"$(printf '%s' "$value" | sed 's/[\\"`$]/\\&/g')\""
      ;;
  esac
  printf '%s' "$value" | sed 's/\\/\\\\/g'
}

# replace_path SRC DEST copies SRC over DEST, swapping it in once copied.
replace_path() {
  rm -rf "$2.new"
  if has ditto; then
    ditto "$1" "$2.new"
  else
    cp -R "$1" "$2.new"
  fi
  rm -rf "$2"
  mv "$2.new" "$2"
}

# link_desktop TARGET links BIN_DIR/srcz-desktop to TARGET.
link_desktop() {
  choose_bin_dir
  ln -sfn "$1" "$BIN_DIR/srcz-desktop"
  ok "linked $BIN_DIR/srcz-desktop"
  if ! on_path "$BIN_DIR"; then
    warn "$BIN_DIR is not on PATH; add it to run srcz-desktop from a terminal"
  fi
}

# remove_electron PATH... deletes each existing PATH of an Electron install.
remove_electron() {
  found=0
  for path in "$@"; do
    if [ -e "$path" ] || [ -L "$path" ]; then
      rm -rf "$path"
      found=1
    fi
  done
  if [ "$found" = 1 ]; then ok "removed the earlier Electron app"; fi
  warn_thunar_action
}

# warn_running_daemon SRCZ warns about a daemon from an earlier build: the app
# reuses it, and shows its web UI, until it stops.
warn_running_daemon() {
  if "$1" daemon status >/dev/null 2>&1; then
    warn "a srcz daemon is running an earlier build; the app shows that build until it stops. To switch now, run:"
    note "\"$1\" daemon stop"
  fi
}

# install_desktop_linux DIR installs the unpacked Linux archive DIR, its
# desktop entry and icon, and makes it the srcz:// handler.
install_desktop_linux() {
  if ! [ -x "$1/srcz-desktop" ] || ! [ -x "$1/bin/srcz" ]; then
    fail "$package has no srcz-desktop app"
  fi
  mkdir -p "$DESKTOP_DATA/srcz-desktop"
  replace_path "$1" "$DESKTOP_HOME"
  version="$("$DESKTOP_HOME/srcz-desktop" --version 2>/dev/null | head -n 1 || true)"
  ok "installed $DESKTOP_HOME ($version)"
  electron_paths remove_electron
  exec_arg="$(desktop_exec "$DESKTOP_HOME/srcz-desktop")"
  mkdir -p "$DESKTOP_DATA/applications" "$(dirname "$DESKTOP_ICON")" "$DESKTOP_CONFIG"
  cp "$DESKTOP_HOME/srcz-desktop.png" "$DESKTOP_ICON"
  cat >"$DESKTOP_DATA/applications/$DESKTOP_ID" <<EOF2
[Desktop Entry]
Type=Application
Name=srcz
Comment=Browse local files with Source Zero
Exec=$exec_arg %U
Icon=srcz-desktop
Terminal=false
Categories=Development;
MimeType=x-scheme-handler/srcz;
EOF2
  if has update-desktop-database; then
    update-desktop-database "$DESKTOP_DATA/applications" >/dev/null 2>&1 || true
  fi
  if has xdg-mime && xdg-mime default "$DESKTOP_ID" x-scheme-handler/srcz; then
    ok "registered srcz:// links"
  else
    warn "could not set the srcz:// handler (xdg-mime); srcz . may serve in a browser instead"
  fi
  link_desktop "$DESKTOP_HOME/srcz-desktop"
  warn_running_daemon "$DESKTOP_HOME/bin/srcz"
}

# install_desktop_macos APP installs srcz-desktop.app into /Applications, or
# ~/Applications when /Applications is not writable, and removes the Electron
# app's srcz.app, which also claims srcz:// links.
install_desktop_macos() {
  apps="/Applications"
  if ! [ -d "$apps" ] || ! [ -w "$apps" ]; then
    apps="$HOME/Applications"
    mkdir -p "$apps"
  fi
  replace_path "$1" "$apps/srcz-desktop.app"
  for dir in /Applications "$HOME/Applications"; do
    [ -e "$dir/srcz.app" ] || continue
    if [ -w "$dir" ]; then
      rm -rf "$dir/srcz.app"
      ok "removed the earlier Electron app $dir/srcz.app"
    else
      warn "$dir/srcz.app is the earlier Electron app and may take srcz:// links; remove it with:"
      note "sudo rm -rf \"$dir/srcz.app\""
    fi
  done
  if [ -x "$LSREGISTER" ]; then
    "$LSREGISTER" -f "$apps/srcz-desktop.app" >/dev/null 2>&1 || true
  fi
  ok "installed $apps/srcz-desktop.app"
  link_desktop "$apps/srcz-desktop.app/Contents/MacOS/srcz-desktop"
  warn_running_daemon "$apps/srcz-desktop.app/Contents/Resources/bin/srcz"
}

# install_desktop_release installs the desktop app of release TAG. It bundles
# the srcz binary it runs the daemon with; the CLI is not installed.
install_desktop_release() {
  if [ "$OS" = linux ]; then
    package="srcz-desktop_${VERSION}_linux_${ARCH}.tar.gz"
  else
    package="srcz-desktop_${VERSION}_darwin_${ARCH}.zip"
  fi

  step "downloading the srcz desktop app $TAG ($OS/$ARCH)"
  fetch "$(release_url "$package")" "$TMP/$package" ||
    fail "srcz $TAG has no $package; earlier releases have no desktop app; it ships in the next stable release and on edge, except macOS Intel"

  mkdir -p "$TMP/desktop"
  if [ "$OS" = linux ]; then
    tar -xzf "$TMP/$package" -C "$TMP/desktop"
    install_desktop_linux "$TMP/desktop"
  else
    if has ditto; then
      ditto -x -k "$TMP/$package" "$TMP/desktop"
    else
      unzip -q "$TMP/$package" -d "$TMP/desktop"
    fi
    [ -d "$TMP/desktop/srcz-desktop.app" ] || fail "$package has no srcz-desktop.app"
    install_desktop_macos "$TMP/desktop/srcz-desktop.app"
  fi
  check_webkit
  ok "done; open a directory in the desktop app with:"
  note "srcz-desktop ."
  note "For srcz . and walkthroughs, install the CLI too: scripts/install.sh"
}
# shared:desktop-install:end

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
    *) fail "could not find the latest srcz release at $url" ;;
  esac
}
# shared:latest:end

# shared:release:begin
# release_url FILE prints FILE's download URL in release TAG.
release_url() {
  printf 'https://github.com/%s/releases/download/%s/%s' "$REPO" "$TAG" "$1"
}

# download_cli sets CLI to the srcz binary of release TAG, whose assets are
# named for VERSION.
download_cli() {
  name="srcz_${VERSION}_${OS}_${ARCH}"
  step "downloading srcz $TAG ($OS/$ARCH)"
  fetch "$(release_url "$name.tar.gz")" "$TMP/$name.tar.gz" || fail "could not download $name.tar.gz from srcz $TAG"
  tar -xzf "$TMP/$name.tar.gz" -C "$TMP"
  CLI="$TMP/$name/srcz"
  [ -f "$CLI" ] || fail "$name.tar.gz has no srcz binary"
}
# shared:release:end

detect_platform
check_desktop_conflicts
make_tmp
latest_release
install_desktop_release
