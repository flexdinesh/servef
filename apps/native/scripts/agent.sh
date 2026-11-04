#!/bin/sh
# Headless agent tooling for the native app (decision 0086). Run from the
# repository root through mise:
#
#   agent.sh build                    build mygo-agent, mygo-compare, native-agent
#   agent.sh serve [flags]            serve the app headless at $MYGO_AGENT_SOCKET
#   agent.sh shots [scenario...]      run scenarios headless (default: all)
#   agent.sh compare [scenario...]    shots, web shots from pnpm dev:mock, composites
#
# Output goes to .dev/native-shots/<scenario>/ (gitignored): <shot>.png,
# tree.json, web/<shot>.png, and compare/<shot>.png.
set -eu

root=$(pwd)
dev="$root/.dev/native-agent"
bin="$dev/bin"
out="${SERVEF_NATIVE_SHOTS:-$root/.dev/native-shots}"
port="${SERVEF_NATIVE_COMPARE_PORT:-5390}"
export MYGO_AGENT_DIR="$dev/state"

build() {
  mkdir -p "$bin"
  GOWORK=off go -C packages/mygo build -o "$bin/" ./cmd/mygo-agent ./cmd/mygo-compare
  GOWORK=off go -C apps/native build -tags agent -o "$bin/native-agent" ./cmd/native-agent
}

# scenario_file resolves a name (document-preview) or a path.
scenario_file() {
  case "$1" in
    */*|*.yaml) printf '%s\n' "$1" ;;
    *) printf '%s\n' "apps/native/scenarios/$1.yaml" ;;
  esac
}

scenario_name() {
  base=${1##*/}
  printf '%s\n' "${base%.yaml}"
}

scenarios() {
  if [ "$#" -eq 0 ]; then
    for file in apps/native/scenarios/*.yaml; do
      printf '%s\n' "$file"
    done
    return
  fi
  for name in "$@"; do
    scenario_file "$name"
  done
}

shots() {
  status=0
  for file in $(scenarios "$@"); do
    name=$(scenario_name "$file")
    "$bin/mygo-agent" --session "$name" open -- "$bin/native-agent"
    if ! "$bin/mygo-agent" --session "$name" run "$file" --out "$out"; then
      status=1
    fi
    "$bin/mygo-agent" --session "$name" close || true
  done
  return "$status"
}

compare() {
  shots "$@"
  log="$dev/dev-mock.log"
  mkdir -p "$dev"
  SERVEF_DEV_WEB_PORT=$port pnpm dev:mock --port "$port" --strictPort >"$log" 2>&1 &
  mock=$!
  # pnpm runs vite as a child; stop both.
  trap 'pkill -P "$mock" 2>/dev/null || true; kill "$mock" 2>/dev/null || true' EXIT INT TERM
  tries=0
  until curl -sf "http://localhost:$port/" >/dev/null 2>&1; do
    tries=$((tries + 1))
    if [ "$tries" -gt 60 ]; then
      printf 'pnpm dev:mock did not start on port %s; see %s\n' "$port" "$log" >&2
      return 1
    fi
    sleep 1
  done
  for file in $(scenarios "$@"); do
    name=$(scenario_name "$file")
    "$bin/mygo-compare" web -base "http://localhost:$port" -out "$out" -session servef-native-compare "$file"
    if [ -d "$out/$name/web" ]; then
      "$bin/mygo-compare" dir -diff "$out/$name"
    else
      printf '%s: no web shots (the scenario has no web section)\n' "$name"
    fi
  done
  agent-browser --session servef-native-compare close >/dev/null 2>&1 || true
}

command=${1:-}
[ "$#" -gt 0 ] && shift
case "$command" in
  build) build ;;
  serve)
    build
    socket="${MYGO_AGENT_SOCKET:-$dev/agent.sock}"
    printf 'drive it with: MYGO_AGENT_SOCKET=%s %s snapshot\n' "$socket" "$bin/mygo-agent"
    exec "$bin/native-agent" --socket "$socket" "$@"
    ;;
  shots) build && shots "$@" ;;
  compare) build && compare "$@" ;;
  *)
    printf 'usage: agent.sh build | serve [flags] | shots [scenario...] | compare [scenario...]\n' >&2
    exit 2
    ;;
esac
