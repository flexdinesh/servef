#!/bin/sh
# What the native app keeps after a large document and a large review close
# (decision 0091), against a baseline. Run from the repository root:
#
#   mise exec -- sh apps/native/scripts/measure-retention.sh [baseline-ref]
#
# Builds native-agent from the working tree and from baseline-ref (default
# origin/main), generates fixtures with a 3.5 MiB Go file and a diff of
# 1 MiB per side, drives each headless through the same steps, and samples
# at every step VmRSS from /proc and the Go heap's live size from gctrace.
# The runtime collects an idle process about every two minutes, so a sample
# waits $SERVEF_RETENTION_SETTLE seconds (default 130) for the next
# collection to show what is still reachable. A baseline without tabs has
# no Close All: its review closes and a small document replaces the large
# one, which is as far as it lets go. Results go to
# .dev/native-retention/results.txt.
set -eu

root=$(pwd)
baseline="${1:-origin/main}"
settle="${SERVEF_RETENTION_SETTLE:-130}"
out="$root/.dev/native-retention"
work=$(mktemp -d)
cleanup() {
  for s in retention-base retention-head; do
    "$work/bin/mygo-agent" --session "$s" close >/dev/null 2>&1 || true
  done
  rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 130' INT TERM
export MYGO_AGENT_DIR="$work/state"
results="$out/results.txt"
mkdir -p "$out" "$work/bin"

GOWORK=off go -C packages/mygo build -o "$work/bin/" ./cmd/mygo-agent
GOWORK=off go -C apps/native build -tags agent -o "$work/bin/head" ./cmd/native-agent
mkdir "$work/base"
git archive "$baseline" apps/native apps/servef packages/mygo testdata | tar -x -C "$work/base"
GOWORK=off go -C "$work/base/apps/native" build -tags agent -o "$work/bin/base" ./cmd/native-agent

cp -R testdata/api/default "$work/fixtures"
node - "$work/fixtures" <<'SCRIPT'
const fs = require("node:fs")
const path = require("node:path")
const dir = process.argv[2]
const read = (name) => JSON.parse(fs.readFileSync(path.join(dir, name), "utf8"))
const write = (name, value) => fs.writeFileSync(path.join(dir, name), JSON.stringify(value))
const text = (bytes, changed) => {
  const lines = []
  let size = 0
  for (let i = 0; size < bytes; i++) {
    const line = `func f${i}() int { return ${i === changed ? -1 : i} } // long enough to count as text`
    lines.push(line)
    size += line.length + 1
  }
  return lines
}
// The large document, under the 4 MiB highlighting limit, in every page's tree.
const add = (nodes, node) => {
  for (const entry of nodes) if (entry.isDir && entry.path === "code") entry.children.push(node)
}
for (const name of fs.readdirSync(dir).filter((n) => n.startsWith("page-"))) {
  const page = read(name)
  add(page.tree, { fileType: "go", name: "large.go", path: "code/large.go", isDir: false, open: false, selected: false })
  write(name, page)
}
const source = text(3.5 * 1024 * 1024, -1).join("\n") + "\n"
write("page-retention.json", { ...read("page-go.json"), tree: read("page-index.json").tree, selected: "code/large.go", source, fileSize: source.length })
// The large diff: one change in a hunk of the whole file.
const count = text(1024 * 1024, -1).length
const middle = Math.floor(count / 2)
const sides = [text(1024 * 1024, -1), text(1024 * 1024, middle)].map((lines) => lines.join("\n") + "\n")
const diff = read("diff-modified.json")
const side = (content) => ({ ...diff.original, content, size: content.length })
write("diff-retention.json", {
  ...diff,
  path: "code/large-change.go",
  original: side(sides[0]),
  modified: side(sides[1]),
  lineDiff: {
    ignoreWhitespace: false, additions: 1, deletions: 1, gaps: [],
    hunks: [{
      oldStart: 1, oldCount: count, newStart: 1, newCount: count, additions: 1, deletions: 1,
      blocks: [
        { kind: "context", oldStart: 1, oldCount: middle, newStart: 1, newCount: middle },
        { kind: "change", oldStart: middle + 1, oldCount: 1, newStart: middle + 1, newCount: 1 },
        { kind: "context", oldStart: middle + 2, oldCount: count - middle - 1, newStart: middle + 2, newCount: count - middle - 1 },
      ],
    }],
  },
})
const changes = read("changes.json")
changes.files.push({ path: "code/large-change.go", status: "modified" })
write("changes.json", changes)
SCRIPT

# sample prints one row: the step, VmRSS, and the live heap of the last
# collection after waiting for one.
sample() {
  step=$1
  wait_for=$2
  sleep "$wait_for"
  rss=$(sed -n 's/^VmRSS:[[:space:]]*\([0-9]*\) kB/\1/p' "/proc/$pid/status")
  live=$(sed -n 's/^gc .* [0-9]*->[0-9]*->\([0-9]*\) MB.*/\1/p' "$log" | tail -n 1)
  printf '%-5s %-22s %7d MiB RSS %5s MiB live\n' "$label" "$step" "$((rss / 1024))" "${live:-?}" | tee -a "$results"
}

run() {
  label=$1
  session="retention-$label"
  agent() { "$work/bin/mygo-agent" --session "$session" "$@"; }
  # Workspace memory came with tabs.
  flags=""
  tabs=0
  if "$work/bin/$label" --help 2>&1 | grep -q -- -memory; then
    flags="--memory $work/$label-workspaces.json"
    tabs=1
  fi
  # shellcheck disable=SC2086 # flags is empty or one flag and its value.
  agent open --replace -- env GODEBUG=gctrace=1 "$work/bin/$label" --fixtures "$work/fixtures" $flags >"$work/$label.open"
  pid=$(sed -n 's/.*"pid": *\([0-9][0-9]*\).*/\1/p' "$MYGO_AGENT_DIR/$session.json")
  log="$MYGO_AGENT_DIR/$session.sock.log"

  agent click markdown >/dev/null
  agent wait --text 'assets' --timeout 20000 >/dev/null
  sample "project open" "$settle"

  agent click code >/dev/null
  agent click large.go >/dev/null
  agent wait --text 'code/large.go' --timeout 60000 >/dev/null
  agent idle --timeout 60000 >/dev/null
  sample "large document" 0
  sample "large document settled" "$settle"

  if [ "$tabs" -eq 1 ]; then agent click "Source Control" >/dev/null; else agent click Changes >/dev/null; fi
  agent wait --text 'code/large-change.go' --timeout 20000 >/dev/null
  agent click code/large-change.go >/dev/null
  agent wait --text 'Ignore whitespace' --timeout 60000 >/dev/null
  agent idle --timeout 60000 >/dev/null
  sample "large review" 0
  sample "large review settled" "$settle"

  if [ "$tabs" -eq 1 ]; then
    agent click "Close code/large.go" --right >/dev/null
    agent menu "Close All" >/dev/null
    agent wait --text 'Review your changes' --timeout 20000 >/dev/null
  else
    agent click Close >/dev/null
    agent click Files >/dev/null
    agent click guides >/dev/null
    agent click getting-started.md >/dev/null
    agent wait --text 'guides/getting-started.md' --timeout 20000 >/dev/null
  fi
  agent idle --timeout 20000 >/dev/null
  sample "closed" 0
  sample "closed settled" "$settle"
  agent close >/dev/null || true
}

: >"$results"
printf 'baseline %s (%s), head %s\n' "$baseline" "$(git rev-parse --short "$baseline")" "$(git rev-parse --short HEAD)" | tee -a "$results"
run base
run head
