#!/bin/sh
# Real-app smoke (decision 0086): the native app built with -tags e2e
# (mygo's automation plugin) on a display, driven by mygo-agent over
# MYGO_AUTOMATION, against the shared fixtures served over TCP. Run it under
# a display and a session bus, as the Dagger native group does:
#
#   xvfb-run -a dbus-run-session -- sh apps/native/scripts/e2e.sh
#
# Writes windows.json, tree.txt, document.png, preview.png, diff.png, restored.png,
# and rss.txt (the app's RSS with the preview open, then with code/main.go's
# diff open in the full-file view) to $SERVEF_NATIVE_E2E_OUT (default .dev/native-e2e).
set -eu

root=$(pwd)
out="${SERVEF_NATIVE_E2E_OUT:-$root/.dev/native-e2e}"
bin="$root/.dev/native-agent/bin"
port="${SERVEF_NATIVE_E2E_PORT:-7979}"
export MYGO_AGENT_DIR="$root/.dev/native-agent/e2e-state"
mkdir -p "$out" "$bin"

sh apps/native/scripts/agent.sh build
GOWORK=off go -C apps/native build -tags e2e -o "$bin/servef-native-e2e" .

"$bin/native-agent" --fixtures-server "127.0.0.1:$port" >"$out/fixtures.log" 2>&1 &
fixtures=$!
trap 'kill "$fixtures" 2>/dev/null || true' EXIT INT TERM

agent() { "$bin/mygo-agent" --session e2e "$@"; }
link="servef-native://127.0.0.1:$port/?project=/home/user/markdown&path=guides/getting-started.md"
# The app may start before the fixtures listen: it shows the error, and
# the smoke fails on the missing document below.
tries=0
until curl -sf "http://127.0.0.1:$port/api/projects" >/dev/null 2>&1; do
  tries=$((tries + 1))
  [ "$tries" -gt 30 ] && { echo "fixtures server did not start" >&2; exit 1; }
  sleep 1
done

# open sets MYGO_AUTOMATION to the session's socket for the app.
# No pipe: set -e sees open fail. The session's record holds the app's pid.
agent open -- "$bin/servef-native-e2e" "$link"
pid=$(sed -n 's/.*"pid":\([0-9][0-9]*\).*/\1/p' "$MYGO_AGENT_DIR/e2e.json")
if [ -z "$pid" ]; then
  echo "native e2e: no app pid in $MYGO_AGENT_DIR/e2e.json" >&2
  exit 1
fi
status=0
# A built app's idle waits for frames only, not for its fetches: wait for
# the texts they bring before reading the tree or capturing.
{
  agent windows >"$out/windows.json" &&
    agent wait --text 'guides/getting-started.md' --timeout 20000 &&
    agent wait --text 'Preview' --timeout 20000 &&
    agent snapshot >"$out/tree.txt" &&
    # The Explorer is a tree: items open and the open document's selected.
    grep -q '^@e[0-9]* tree "Files"' "$out/tree.txt" &&
    grep -q '^@e[0-9]* treeItem "guides" .* expanded' "$out/tree.txt" &&
    grep -q '^@e[0-9]* treeItem "getting-started.md" .* checked' "$out/tree.txt" &&
    agent screenshot "$out/document.png" &&
    agent click Preview &&
    agent wait --text 'frontend-only fixture API' --timeout 20000 &&
    agent idle --timeout 20000 &&
    agent snapshot >"$out/tree-preview.txt" &&
    agent screenshot "$out/preview.png" &&
    sed -n 's/^VmRSS/preview VmRSS/p' "/proc/$pid/status" >"$out/rss.txt" &&
    agent click "Source Control" &&
    agent wait --text 'code/main.go' --timeout 20000 &&
    agent click code/main.go &&
    agent wait --text 'R7' --timeout 20000 &&
    agent click 'Full file' &&
    agent idle --timeout 20000 &&
    agent screenshot "$out/diff.png" &&
    sed -n -e 's/^VmRSS/diff VmRSS/p' -e 's/^VmHWM/diff VmHWM/p' "/proc/$pid/status" >>"$out/rss.txt" &&
    cat "$out/rss.txt" &&
    agent click Explorer &&
    agent click config &&
    agent click settings.json &&
    agent wait --text 'config/settings.json' --timeout 20000
} || status=1
cp "$MYGO_AGENT_DIR"/e2e.sock.log "$out/app.log" 2>/dev/null || true
agent close || true

# Workspace memory (decision 0091): a new run of the app, linked to the
# project alone, restores its tabs, active tab, and open folders from the
# user data dir.
if [ "$status" -eq 0 ]; then
  agent open -- "$bin/servef-native-e2e" "servef-native://127.0.0.1:$port/?project=/home/user/markdown"
  {
    agent wait --text 'config/settings.json' --timeout 20000 &&
      agent snapshot >"$out/tree-restored.txt" &&
      grep -q '^@e[0-9]* tab "guides/getting-started.md"' "$out/tree-restored.txt" &&
      grep -q '^@e[0-9]* tab "Uncommitted changes"' "$out/tree-restored.txt" &&
      grep -q '^@e[0-9]* tab "config/settings.json" .*checked' "$out/tree-restored.txt" &&
      grep -q '^@e[0-9]* button "Close config/settings.json"' "$out/tree-restored.txt" &&
      grep -q '^@e[0-9]* treeItem "guides" .* expanded' "$out/tree-restored.txt" &&
      grep -q '^@e[0-9]* treeItem "settings.json" .* checked' "$out/tree-restored.txt" &&
      agent screenshot "$out/restored.png"
  } || status=1
  cp "$MYGO_AGENT_DIR"/e2e.sock.log "$out/app-restored.log" 2>/dev/null || true
  agent close || true
fi
if [ "$status" -ne 0 ]; then
  echo "native e2e smoke failed; see $out" >&2
  cat "$out/app.log" >&2 2>/dev/null || true
fi
exit "$status"
