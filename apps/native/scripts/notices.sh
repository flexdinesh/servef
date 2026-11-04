#!/bin/sh
# Writes THIRD_PARTY_NOTICES.txt for the native app's release packages: the
# licenses of the WebAssembly modules it embeds (decision 0088;
# @pierre/highlights, Apache-2.0, with its NOTICE; ondine, MIT), of
# wazero (Apache-2.0), which runs them, and of @pierre/trees' file icons
# (decision 0090; Apache-2.0, with its NOTICE), and of the Lucide icons
# the workspace shell draws (ISC, with Feather's MIT).
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
trees_version="$(sed -n 's/^const PierreVersion = "\(.*\)"$/\1/p' "$mygo/trees/pierre_icons.go")"
lucide_version="$(sed -n 's/^const LucideVersion = "\(.*\)"$/\1/p' internal/view/lucide_icons.go)"

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
  section "@pierre/trees $trees_version (file icons): LICENSE.md" "$mygo/trees/PIERRE-LICENSE.md"
  section "@pierre/trees $trees_version: NOTICE.md" "$mygo/trees/PIERRE-NOTICE.md"
  section "lucide-react $lucide_version (icons): LICENSE" internal/view/LUCIDE-LICENSE
} >"$out"
