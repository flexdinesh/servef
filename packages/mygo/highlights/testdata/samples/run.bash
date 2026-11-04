#!/usr/bin/env bash
# Shell sample
set -euo pipefail
name=${1:-world}
greet() {
  local n="$1"
  echo "hi $n" | tr a-z A-Z >&2
}
for f in *.txt; do [ -f "$f" ] && greet "$f"; done
export PATH="$HOME/bin:$PATH"; cat <<'X'
heredoc $not
X
