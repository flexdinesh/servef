#!/bin/sh
# Installs the servef desktop app and CLI built from the edge branch (latest
# main with the built frontend) on this machine. For development; stable
# installs use scripts/install-desktop.sh. Decision 0073.
#
#   curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-desktop-edge.sh | sh
#
# Needs git and mise (Go, Node, and pnpm come from the repository's mise.lock);
# linux/amd64 and darwin/arm64 only. Linux: the app goes to
# ~/.local/share/servef-desktop/app with a desktop entry for servef:// links,
# an icon, and Dolphin and Nemo "Open in servef" actions. macOS: servef.app in
# /Applications (or ~/Applications). Remove with scripts/uninstall.sh.

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
APP_HOME="$DATA_HOME/servef-desktop"
DESKTOP_ID="servef-desktop.desktop"
ICON_PATH="$DATA_HOME/icons/hicolor/512x512/apps/servef-desktop.png"
DOLPHIN_ACTION="$DATA_HOME/kio/servicemenus/servef-desktop.desktop"
NEMO_ACTION="$DATA_HOME/nemo/actions/servef-desktop.nemo_action"
DEB_EXECUTABLE="/opt/servef/servef-desktop"

app_running() {
  has pgrep || return 1
  if [ "$OS" = darwin ]; then
    pgrep -f 'servef\.app/Contents/MacOS/servef' >/dev/null 2>&1
  else
    pgrep -x -u "$(id -u)" servef-desktop >/dev/null 2>&1
  fi
}

check_desktop_conflicts() {
  if [ "$OS" = linux ] && [ -e "$DEB_EXECUTABLE" ]; then
    fail "the servef-desktop deb is installed ($DEB_EXECUTABLE); keep using it, or remove it first: sudo apt remove servef-desktop"
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
Icon=servef-desktop
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
Icon=servef-desktop
Exec=$exec_arg --open-path %F
EOF
  chmod 755 "$DOLPHIN_ACTION"
  cat >"$NEMO_ACTION" <<EOF
[Nemo Action]
Name=Open in servef
Comment=Open the selected folders in servef
Exec=$exec_arg --open-path %F
Icon-Name=servef-desktop
Selection=notnone
Extensions=dir;
EOF
  if has update-desktop-database; then
    update-desktop-database "$DATA_HOME/applications" >/dev/null 2>&1 || true
  fi
  if has xdg-mime && xdg-mime default "$DESKTOP_ID" x-scheme-handler/servef; then
    ok "registered servef:// links and Open in servef (Dolphin, Nemo)"
  else
    warn "could not set the servef:// handler (xdg-mime); servef . may serve in a browser instead"
  fi
}

# install_linux_dir DIR installs an unpacked Linux app; sets APP_EXEC.
install_linux_dir() {
  mkdir -p "$APP_HOME"
  replace_path "$1" "$APP_HOME/app"
  rm -f "$APP_HOME/servef-desktop.AppImage"
  APP_EXEC="$APP_HOME/app/servef-desktop"
  ok "installed $APP_HOME/app"
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

finish_desktop() {
  ok "done; open a directory in the app with:"
  note "servef ."
}
# shared:desktop-install:end

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
case "$OS/$ARCH" in
  linux/amd64 | darwin/arm64) ;;
  *) fail "edge desktop builds support linux/amd64 and darwin/arm64 (the mise.lock platforms), not $OS/$ARCH" ;;
esac
has mise || fail "mise is required to build the desktop app; install it with: curl https://mise.run | sh"
check_desktop_conflicts
make_tmp
clone_edge

step "installing build tools and dependencies"
(cd "$SRC" && mise trust mise.toml && mise install && mise exec -- pnpm install --frozen-lockfile)
step "building the desktop app ($OS/$ARCH)"
(cd "$SRC" && mise run package:desktop:dir)

if [ "$OS" = linux ]; then
  app="$SRC/desktop/dist/linux-unpacked"
else
  app="$SRC/desktop/dist/mac-arm64/servef.app"
fi
[ -d "$app" ] || fail "the build did not produce $app"

if [ "$OS" = linux ]; then
  install_linux_dir "$app"
  register_linux_app "$APP_EXEC" "$SRC/desktop/resources/icon.png"
else
  install_macos_app "$app"
fi
install_cli "$SRC/desktop/.build/servef-host"
finish_desktop
