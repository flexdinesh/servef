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
# One browser session per checkout: worktrees comparing at once must not
# drive each other's pages.
session="servef-native-compare-$(printf '%s' "$root" | cksum | cut -d ' ' -f 1)"

# Callers in && or || lists turn set -e off inside functions: every step
# returns its own failure.
build() {
  mkdir -p "$bin" || return 1
  GOWORK=off go -C packages/mygo build -o "$bin/" ./cmd/mygo-agent ./cmd/mygo-compare || return 1
  GOWORK=off go -C apps/native build -tags agent -o "$bin/native-agent" ./cmd/native-agent || return 1
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
    "$bin/mygo-agent" --session "$name" open --replace -- "$bin/native-agent" || return 1
    if ! "$bin/mygo-agent" --session "$name" run "$file" --out "$out"; then
      status=1
    fi
    "$bin/mygo-agent" --session "$name" close || true
  done
  return "$status"
}

# listening reports whether anything accepts TCP connections on port, on
# IPv4 or IPv6 loopback, whatever it answers.
listening() {
  node -e '
    const net = require("node:net");
    let left = 2;
    for (const host of ["127.0.0.1", "::1"]) {
      const s = net.connect(Number(process.argv[1]), host);
      s.on("connect", () => process.exit(0));
      s.on("error", () => { if (--left === 0) process.exit(1); });
    }
  ' "$1"
}

compare() {
  shots "$@" || return 1
  log="$dev/dev-mock.log"
  mkdir -p "$dev" || return 1
  if listening "$port"; then
    printf 'port %s is in use (another compare?); set SERVEF_NATIVE_COMPARE_PORT\n' "$port" >&2
    return 1
  fi
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
    "$bin/mygo-compare" web -base "http://localhost:$port" -out "$out" -session "$session" "$file" || return 1
    if [ -d "$out/$name/web" ]; then
      "$bin/mygo-compare" dir -diff "$out/$name" || return 1
    else
      printf '%s: no web shots (the scenario has no web section)\n' "$name"
    fi
  done
  agent-browser --session "$session" close >/dev/null 2>&1 || true
}

command=${1:-}
[ "$#" -gt 0 ] && shift
case "$command" in
  build) build ;;
  serve)
    build || exit 1
    socket="${MYGO_AGENT_SOCKET:-$dev/agent.sock}"
    printf 'drive it with: MYGO_AGENT_SOCKET=%s %s snapshot\n' "$socket" "$bin/mygo-agent"
    exec "$bin/native-agent" --socket "$socket" "$@"
    ;;
  shots)
    build || exit 1
    shots "$@" || exit 1
    ;;
  compare)
    build || exit 1
    compare "$@" || exit 1
    ;;
  *)
    printf 'usage: agent.sh build | serve [flags] | shots [scenario...] | compare [scenario...]\n' >&2
    exit 2
    ;;
esac
