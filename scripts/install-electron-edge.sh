#!/bin/sh
# Installs the servef desktop app and CLI from the edge prerelease: latest
# main, rebuilt by CI after every green push. Stable installs use
# scripts/install-electron.sh. Decisions 0073 and 0079.
#
#   curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-electron-edge.sh | sh
#
# Installs in the same places as scripts/install-electron.sh and replaces
# either install. Remove with scripts/uninstall.sh.

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

# shared:desktop-install:begin
LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

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

# remove_old_install deletes what the scripts installed as servef-desktop
# (decision 0092). The login entry stays: the app moves it when it starts.
remove_old_install() {
  if [ -e "$OLD_APP_HOME" ] || [ -e "$DATA_HOME/applications/$OLD_DESKTOP_ID" ]; then
    ok "replaced the earlier servef-desktop install"
  fi
  rm -rf "$OLD_APP_HOME" "$DATA_HOME/applications/$OLD_DESKTOP_ID" "$OLD_ICON_PATH" "$OLD_DOLPHIN_ACTION" "$OLD_NEMO_ACTION"
}

# register_linux_app EXEC ICON writes the desktop entry, icon, and file
# manager actions for EXEC, and makes it the servef:// handler.
register_linux_app() {
  exec_arg="$(desktop_exec "$1")"
  mkdir -p "$DATA_HOME/applications" "$(dirname "$ICON_PATH")" "$(dirname "$DOLPHIN_ACTION")" "$(dirname "$NEMO_ACTION")"
  cp "$2" "$ICON_PATH"
  cat >"$DATA_HOME/applications/$DESKTOP_ID" <<EOF
[Desktop Entry]
Type=Application
Name=servef
Comment=Browse local files with servef
Exec=$exec_arg %U
Icon=servef-electron
Terminal=false
Categories=Development;
MimeType=x-scheme-handler/servef;
EOF
  cat >"$DOLPHIN_ACTION" <<EOF
[Desktop Entry]
Type=Service
MimeType=inode/directory;
Actions=openInServef;
X-KDE-Priority=TopLevel

[Desktop Action openInServef]
Name=Open in servef
Icon=servef-electron
Exec=$exec_arg --open-path %F
EOF
  chmod 755 "$DOLPHIN_ACTION"
  cat >"$NEMO_ACTION" <<EOF
[Nemo Action]
Name=Open in servef
Comment=Open the selected folders in servef
Exec=$exec_arg --open-path %F
Icon-Name=servef-electron
Selection=notnone
Extensions=dir;
EOF
  remove_old_install
  if has update-desktop-database; then
    update-desktop-database "$DATA_HOME/applications" >/dev/null 2>&1 || true
  fi
  if has xdg-mime && xdg-mime default "$DESKTOP_ID" x-scheme-handler/servef; then
    ok "registered servef:// links and Open in servef (Dolphin, Nemo)"
  else
    warn "could not set the servef:// handler (xdg-mime); servef . may serve in a browser instead"
  fi
}

# install_linux_appimage FILE installs a Linux AppImage; sets APP_EXEC.
install_linux_appimage() {
  mkdir -p "$APP_HOME"
  staged="$APP_HOME/.servef-electron.AppImage.$$"
  cp "$1" "$staged"
  chmod 755 "$staged"
  mv -f "$staged" "$APP_HOME/servef-electron.AppImage"
  rm -rf "$APP_HOME/app"
  APP_EXEC="$APP_HOME/servef-electron.AppImage"
  ok "installed $APP_EXEC"
  for lib in /usr/lib*/libfuse.so.2 /usr/lib/*/libfuse.so.2 /lib*/libfuse.so.2 /lib/*/libfuse.so.2; do
    if [ -e "$lib" ]; then return 0; fi
  done
  warn "libfuse2 was not found; if the AppImage does not start, install FUSE 2:"
  note "Debian/Ubuntu: sudo apt install libfuse2t64 (or libfuse2)"
  note "Fedora: sudo dnf install fuse-libs   Arch: sudo pacman -S fuse2"
}

# install_macos_app APP installs servef.app into /Applications, or
# ~/Applications when /Applications is not writable.
install_macos_app() {
  apps="/Applications"
  if ! [ -d "$apps" ] || ! [ -w "$apps" ]; then
    apps="$HOME/Applications"
    mkdir -p "$apps"
  fi
  replace_path "$1" "$apps/servef.app"
  if [ -x "$LSREGISTER" ]; then
    "$LSREGISTER" -f "$apps/servef.app" >/dev/null 2>&1 || true
  fi
  ok "installed $apps/servef.app"
  if [ "$apps" != /Applications ] && [ -e /Applications/servef.app ]; then
    warn "/Applications/servef.app is another copy; remove it so servef:// links open $apps/servef.app"
  fi
}

# install_desktop_release installs the desktop app and CLI of release TAG.
install_desktop_release() {
  if [ "$OS" = linux ]; then
    package="servef-electron_${VERSION}_linux_${ARCH}.AppImage"
  else
    package="servef-electron_${VERSION}_darwin_${ARCH}.zip"
  fi

  step "downloading the servef desktop app $TAG ($OS/$ARCH)"
  fetch "$(release_url "$package")" "$TMP/$package" ||
    fail "servef $TAG has no $package; releases before desktop packages have none"
  download_cli

  if [ "$OS" = linux ]; then
    install_linux_appimage "$TMP/$package"
    # Releases before the rename keep the icon under apps/desktop/, and before
    # the apps/ layout under desktop/.
    fetch "https://raw.githubusercontent.com/$REPO/$TAG/apps/electron/resources/icon.png" "$TMP/icon.png" ||
      fetch "https://raw.githubusercontent.com/$REPO/$TAG/apps/desktop/resources/icon.png" "$TMP/icon.png" ||
      fetch "https://raw.githubusercontent.com/$REPO/$TAG/desktop/resources/icon.png" "$TMP/icon.png" ||
      fail "could not download the servef icon for $TAG"
    register_linux_app "$APP_EXEC" "$TMP/icon.png"
  else
    mkdir -p "$TMP/app"
    if has ditto; then
      ditto -x -k "$TMP/$package" "$TMP/app"
    else
      unzip -q "$TMP/$package" -d "$TMP/app"
    fi
    [ -d "$TMP/app/servef.app" ] || fail "$package has no servef.app"
    install_macos_app "$TMP/app/servef.app"
  fi
  install_cli "$CLI"
  ok "done; open a directory in the app with:"
  note "servef ."
}
# shared:desktop-install:end

# shared:edge:begin
# edge_release selects the edge prerelease: after every green push to main, CI
# replaces its assets with that commit's build, named for version 0.0.0-edge.
edge_release() {
  TAG=edge
  VERSION=0.0.0-edge
}
# shared:edge:end

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
check_desktop_conflicts
make_tmp
edge_release
install_desktop_release
