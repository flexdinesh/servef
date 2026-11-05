#!/bin/sh
# Moved to install-electron.sh (decision 0092). Builds and docs from before the rename
# print this URL, so it hands over instead of failing.
set -eu

URL="https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-electron.sh"

if command -v curl >/dev/null 2>&1; then
  script="$(curl -fsSL "$URL")"
elif command -v wget >/dev/null 2>&1; then
  script="$(wget -q -O - "$URL")"
else
  printf ' curl or wget is required\n' >&2
  exit 1
fi
exec sh -c "$script"
