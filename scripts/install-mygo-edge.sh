#!/bin/sh
# Installs the experimental servef mygo app (decision 0094) from the edge
# prerelease: latest main, rebuilt by CI after every green push. Stable
# installs use scripts/install-mygo.sh. Decisions 0079 and 0094.
#
#   curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-mygo-edge.sh | sh
#
# Installs in the same places as scripts/install-mygo.sh and replaces
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

# shared:mygo:begin
MYGO_DATA="${XDG_DATA_HOME:-}"
case "$MYGO_DATA" in
  /*) ;;
  *) MYGO_DATA="$HOME/.local/share" ;;
esac
# The app has a directory of its own: WebKit keeps the page's localStorage
# beside it in servef-mygo/, which updates and uninstall leave alone.
MYGO_HOME="$MYGO_DATA/servef-mygo/app"
MYGO_DESKTOP_ID="servef-mygo.desktop"
MYGO_ICON="$MYGO_DATA/icons/hicolor/512x512/apps/servef-mygo.png"

mygo_running() {
  has pgrep || return 1
  if [ "$OS" = darwin ]; then
    pgrep -f 'servef-mygo\.app/Contents/MacOS/servef-mygo' >/dev/null 2>&1
  else
    pgrep -x -u "$(id -u)" servef-mygo >/dev/null 2>&1
  fi
}
# shared:mygo:end

# shared:mygo-install:begin
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

MYGO_LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

check_mygo_conflicts() {
  if mygo_running; then
    fail "the servef mygo app is running; quit it, then run this again"
  fi
}

# mygo_exec PATH renders PATH as one desktop entry Exec argument.
mygo_exec() {
  value="$(printf '%s' "$1" | sed 's/%/%%/g')"
  case "$value" in
    *[!A-Za-z0-9_./+,:@=-]*)
      # shellcheck disable=SC2016 # sed expression, not shell expansion
      value="\"$(printf '%s' "$value" | sed 's/[\\"`$]/\\&/g')\""
      ;;
  esac
  printf '%s' "$value" | sed 's/\\/\\\\/g'
}

# mygo_replace SRC DEST copies SRC over DEST, swapping it in once copied.
mygo_replace() {
  rm -rf "$2.new"
  if has ditto; then
    ditto "$1" "$2.new"
  else
    cp -R "$1" "$2.new"
  fi
  rm -rf "$2"
  mv "$2.new" "$2"
}

# link_mygo TARGET links BIN_DIR/servef-mygo to TARGET.
link_mygo() {
  choose_bin_dir
  ln -sfn "$1" "$BIN_DIR/servef-mygo"
  ok "linked $BIN_DIR/servef-mygo"
  if ! on_path "$BIN_DIR"; then
    warn "$BIN_DIR is not on PATH; add it to run servef-mygo from a terminal"
  fi
}

# install_mygo_linux DIR installs the unpacked Linux archive DIR, its
# desktop entry and icon, and makes it the servef-mygo:// handler.
install_mygo_linux() {
  if ! [ -x "$1/servef-mygo" ] || ! [ -x "$1/bin/servef" ]; then
    fail "$package has no servef-mygo app"
  fi
  mkdir -p "$MYGO_DATA/servef-mygo"
  mygo_replace "$1" "$MYGO_HOME"
  version="$("$MYGO_HOME/servef-mygo" --version 2>/dev/null | head -n 1 || true)"
  ok "installed $MYGO_HOME ($version)"
  exec_arg="$(mygo_exec "$MYGO_HOME/servef-mygo")"
  mkdir -p "$MYGO_DATA/applications" "$(dirname "$MYGO_ICON")"
  cp "$MYGO_HOME/servef-mygo.png" "$MYGO_ICON"
  cat >"$MYGO_DATA/applications/$MYGO_DESKTOP_ID" <<EOF2
[Desktop Entry]
Type=Application
Name=servef mygo (experimental)
Comment=Browse local files with servef (experimental mygo app)
Exec=$exec_arg %U
Icon=servef-mygo
Terminal=false
Categories=Development;
MimeType=x-scheme-handler/servef-mygo;
EOF2
  if has update-desktop-database; then
    update-desktop-database "$MYGO_DATA/applications" >/dev/null 2>&1 || true
  fi
  if has xdg-mime && xdg-mime default "$MYGO_DESKTOP_ID" x-scheme-handler/servef-mygo; then
    ok "registered servef-mygo:// links"
  else
    warn "could not set the servef-mygo:// handler (xdg-mime)"
  fi
  link_mygo "$MYGO_HOME/servef-mygo"
}

# install_mygo_macos APP installs servef-mygo.app into /Applications, or
# ~/Applications when /Applications is not writable.
install_mygo_macos() {
  apps="/Applications"
  if ! [ -d "$apps" ] || ! [ -w "$apps" ]; then
    apps="$HOME/Applications"
    mkdir -p "$apps"
  fi
  mygo_replace "$1" "$apps/servef-mygo.app"
  if [ -x "$MYGO_LSREGISTER" ]; then
    "$MYGO_LSREGISTER" -f "$apps/servef-mygo.app" >/dev/null 2>&1 || true
  fi
  ok "installed $apps/servef-mygo.app"
  link_mygo "$apps/servef-mygo.app/Contents/MacOS/servef-mygo"
}

# install_mygo_release installs the mygo app of release TAG. It bundles
# the servef binary it runs the daemon with; the CLI is not installed.
install_mygo_release() {
  if [ "$OS" = linux ]; then
    package="servef-mygo_${VERSION}_linux_${ARCH}.tar.gz"
  else
    package="servef-mygo_${VERSION}_darwin_${ARCH}.zip"
  fi

  step "downloading the servef mygo app $TAG ($OS/$ARCH, experimental)"
  fetch "$(release_url "$package")" "$TMP/$package" ||
    fail "servef $TAG has no $package; releases before the mygo app have none"

  mkdir -p "$TMP/mygo"
  if [ "$OS" = linux ]; then
    tar -xzf "$TMP/$package" -C "$TMP/mygo"
    install_mygo_linux "$TMP/mygo"
  else
    if has ditto; then
      ditto -x -k "$TMP/$package" "$TMP/mygo"
    else
      unzip -q "$TMP/$package" -d "$TMP/mygo"
    fi
    [ -d "$TMP/mygo/servef-mygo.app" ] || fail "$package has no servef-mygo.app"
    install_mygo_macos "$TMP/mygo/servef-mygo.app"
  fi
  check_webkit
  ok "done; open a directory in the mygo app with:"
  note "servef-mygo ."
}
# shared:mygo-install:end

# shared:edge:begin
# edge_release selects the edge prerelease: after every green push to main, CI
# replaces its assets with that commit's build, named for version 0.0.0-edge.
# Decision 0093: macOS Intel is stable only.
edge_release() {
  if [ "$OS" = darwin ] && [ "$ARCH" = amd64 ]; then
    fail "edge has no macOS Intel builds; use the stable install script (the name without -edge)"
  fi
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
check_mygo_conflicts
make_tmp
edge_release
install_mygo_release
