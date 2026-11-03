#!/bin/sh
# Installs the experimental servef native app (decision 0082) from the latest
# release. It bundles the servef binary it starts the daemon with; the CLI is
# installed separately (scripts/install.sh).
#
#   curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-native.sh | sh
#
# Linux: the app goes to ~/.local/share/servef-native with a desktop entry for
# servef-native:// links and an icon. macOS: servef-native.app in
# /Applications (or ~/Applications). servef-native goes on PATH in
# ~/.local/bin (or ~/bin when that is on PATH). Latest main:
# scripts/install-native-edge.sh. Remove with scripts/uninstall.sh.

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

# shared:native-install:begin
NATIVE_LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

check_native_conflicts() {
  if native_running; then
    fail "the servef native app is running; quit it, then run this again"
  fi
}

# native_exec PATH renders PATH as one desktop entry Exec argument.
native_exec() {
  value="$(printf '%s' "$1" | sed 's/%/%%/g')"
  case "$value" in
    *[!A-Za-z0-9_./+,:@=-]*)
      # shellcheck disable=SC2016 # sed expression, not shell expansion
      value="\"$(printf '%s' "$value" | sed 's/[\\"`$]/\\&/g')\""
      ;;
  esac
  printf '%s' "$value" | sed 's/\\/\\\\/g'
}

# native_replace SRC DEST copies SRC over DEST, swapping it in once copied.
native_replace() {
  rm -rf "$2.new"
  if has ditto; then
    ditto "$1" "$2.new"
  else
    cp -R "$1" "$2.new"
  fi
  rm -rf "$2"
  mv "$2.new" "$2"
}

# link_native TARGET links BIN_DIR/servef-native to TARGET.
link_native() {
  choose_bin_dir
  ln -sfn "$1" "$BIN_DIR/servef-native"
  ok "linked $BIN_DIR/servef-native"
  if ! on_path "$BIN_DIR"; then
    warn "$BIN_DIR is not on PATH; add it to run servef-native from a terminal"
  fi
}

# install_native_linux DIR installs the unpacked Linux archive DIR, its
# desktop entry and icon, and makes it the servef-native:// handler.
install_native_linux() {
  if ! [ -x "$1/servef-native" ] || ! [ -x "$1/bin/servef" ]; then
    fail "$package has no servef-native app"
  fi
  mkdir -p "$NATIVE_DATA"
  native_replace "$1" "$NATIVE_HOME"
  version="$("$NATIVE_HOME/servef-native" --version 2>/dev/null | head -n 1 || true)"
  ok "installed $NATIVE_HOME ($version)"
  exec_arg="$(native_exec "$NATIVE_HOME/servef-native")"
  mkdir -p "$NATIVE_DATA/applications" "$(dirname "$NATIVE_ICON")"
  cp "$NATIVE_HOME/servef-native.png" "$NATIVE_ICON"
  cat >"$NATIVE_DATA/applications/$NATIVE_DESKTOP_ID" <<EOF2
[Desktop Entry]
Type=Application
Name=servef native (experimental)
Comment=Browse local files with servef (experimental native app)
Exec=$exec_arg %U
Icon=servef-native
Terminal=false
Categories=Development;
MimeType=x-scheme-handler/servef-native;
EOF2
  if has update-desktop-database; then
    update-desktop-database "$NATIVE_DATA/applications" >/dev/null 2>&1 || true
  fi
  if has xdg-mime && xdg-mime default "$NATIVE_DESKTOP_ID" x-scheme-handler/servef-native; then
    ok "registered servef-native:// links"
  else
    warn "could not set the servef-native:// handler (xdg-mime)"
  fi
  link_native "$NATIVE_HOME/servef-native"
}

# install_native_macos APP installs servef-native.app into /Applications, or
# ~/Applications when /Applications is not writable.
install_native_macos() {
  apps="/Applications"
  if ! [ -d "$apps" ] || ! [ -w "$apps" ]; then
    apps="$HOME/Applications"
    mkdir -p "$apps"
  fi
  native_replace "$1" "$apps/servef-native.app"
  if [ -x "$NATIVE_LSREGISTER" ]; then
    "$NATIVE_LSREGISTER" -f "$apps/servef-native.app" >/dev/null 2>&1 || true
  fi
  ok "installed $apps/servef-native.app"
  link_native "$apps/servef-native.app/Contents/MacOS/servef-native"
}

# install_native_release installs the native app of release TAG. It bundles
# the servef binary it runs the daemon with; the CLI is not installed.
install_native_release() {
  if [ "$OS" = linux ]; then
    package="servef-native_${VERSION}_linux_${ARCH}.tar.gz"
  else
    package="servef-native_${VERSION}_darwin_${ARCH}.zip"
  fi

  step "downloading the servef native app $TAG ($OS/$ARCH, experimental)"
  fetch "$(release_url "$package")" "$TMP/$package" ||
    fail "servef $TAG has no $package; releases before the native app have none"

  mkdir -p "$TMP/native"
  if [ "$OS" = linux ]; then
    tar -xzf "$TMP/$package" -C "$TMP/native"
    install_native_linux "$TMP/native"
  else
    if has ditto; then
      ditto -x -k "$TMP/$package" "$TMP/native"
    else
      unzip -q "$TMP/$package" -d "$TMP/native"
    fi
    [ -d "$TMP/native/servef-native.app" ] || fail "$package has no servef-native.app"
    install_native_macos "$TMP/native/servef-native.app"
  fi
  ok "done; open a directory in the native app with:"
  note "servef-native ."
}
# shared:native-install:end

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
check_native_conflicts
make_tmp
latest_release
install_native_release
