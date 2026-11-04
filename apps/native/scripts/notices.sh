#!/bin/sh
# Writes THIRD_PARTY_NOTICES.txt for the native app's release packages: the
# licenses of the WebAssembly modules it embeds (decision 0088;
# @pierre/highlights, Apache-2.0, with its NOTICE; ondine, MIT) and of
# wazero (Apache-2.0), which runs them.
#
#   apps/native/scripts/notices.sh OUT
#
# Runs from apps/native, with the module's dependencies downloaded.
set -eu
export GOWORK=off

out="$1"
mygo=../../packages/mygo
wazero="$(go list -m -f '{{.Dir}}' github.com/tetratelabs/wazero)"
wazero_version="$(go list -m -f '{{.Version}}' github.com/tetratelabs/wazero)"
highlights_version="$(sed -n 's/^	Version = "\(.*\)"$/\1/p' "$mygo/highlights/pin.go")"
ondine_version="$(sed -n 's/^	Version = "\(.*\)"$/\1/p' "$mygo/ondine/pin.go")"

section() {
  printf '%s\n%s\n\n' "================================================================" "$1"
  cat "$2"
  printf '\n'
}

{
  printf 'servef-native includes the following third-party software.\n\n'
  section "@pierre/highlights $highlights_version (highlights.wasm): LICENSE.md" "$mygo/highlights/LICENSE.md"
  section "@pierre/highlights $highlights_version: NOTICE.md" "$mygo/highlights/NOTICE.md"
  section "ondine $ondine_version (all.generated.wasm): LICENSE" "$mygo/ondine/LICENSE"
  section "github.com/tetratelabs/wazero $wazero_version: LICENSE" "$wazero/LICENSE"
} >"$out"
