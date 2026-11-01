// Local validation in private Linux containers with immutable preparation.
package main

import (
	"context"
	"dagger/servef-validation/internal/dagger"
	"encoding/json"
	"fmt"
	"strings"
)

type ServefValidation struct{}

const baseImage = "debian:bookworm-slim@sha256:a3bba7dc7dbcc1f37c2856c51fc9a7fac96608f50cb87b1b4e4d8b8c85ae988e"

// Run returns diagnostics and status.json even for failed checks. The caller
// must inspect exitCode before issuing a successful validation receipt.
func (m *ServefValidation) Run(ctx context.Context,
	// +ignore=[".git", "node_modules", "web/node_modules", "site/node_modules", "internal/web/dist", "site/dist", ".scratch", ".dev", ".env", "test-results", "playwright-report"]
	source *dagger.Directory, group string, attempt string,
	// +optional
	force bool,
	// +optional
	// +default="[]"
	argsJSON string,
	// +optional
	task string,
) (*dagger.Directory, error) {
	groups := map[string]bool{"go": true, "frontend": true, "tooling": true, "docs": true, "browser": true}
	if !groups[group] || attempt == "" {
		return nil, fmt.Errorf("valid group and nonempty attempt required")
	}
	owners := map[string]string{
		"fmt:check": "go", "tidy:check": "go", "vet": "go", "test:go": "go", "test:race": "go",
		"test:web": "frontend", "typecheck:web": "frontend", "lint": "frontend",
		"test:release": "tooling", "typecheck:root": "tooling", "mise:check": "tooling",
		"typecheck:site": "docs", "build:site": "docs", "test:browser": "browser",
	}
	task = strings.TrimPrefix(task, "_validate:")
	if task != "" && owners[task] != group {
		return nil, fmt.Errorf("task %q does not belong to group %q", task, group)
	}
	var args []string
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("args-json: %w", err)
	}
	prepared := dependencies(source)
	runner := prepared.WithDirectory("/src", source)
	if group == "go" || group == "browser" {
		web := prepared.WithDirectory("/src", subset(source, []string{"web/**", "patches/**", "mise.toml", "mise.lock", ".node-version", "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "site/package.json"}))
		if force {
			web = web.WithEnvVariable("SERVEF_BUILD_ATTEMPT", attempt)
		}
		web = web.WithExec([]string{"mise", "run", "--force", "build:web"})
		runner = runner.WithDirectory("/src/internal/web/dist", web.Directory("/src/internal/web/dist"))
		modules := toolchain(source).WithDirectory("/src", subset(source, []string{"go.mod", "go.sum"})).WithExec([]string{"mise", "exec", "--", "go", "mod", "download"})
		runner = runner.WithDirectory("/home/runner/go/pkg/mod", modules.Directory("/home/runner/go/pkg/mod"))
	}
	if group == "browser" {
		binary := prepared.WithDirectory("/src", subset(source, []string{"*.go", "go.mod", "go.sum", "cmd/**", "internal/**"})).
			WithDirectory("/src/internal/web/dist", runner.Directory("/src/internal/web/dist")).
			WithDirectory("/home/runner/go/pkg/mod", runner.Directory("/home/runner/go/pkg/mod"))
		if force {
			binary = binary.WithEnvVariable("SERVEF_BUILD_ATTEMPT", attempt)
		}
		binary = binary.WithExec([]string{"mise", "run", "--force", "_validate:binary"})
		browser := prepared.WithExec([]string{"mise", "exec", "--", "pnpm", "exec", "playwright", "install", "--with-deps", "chromium"})
		runner = browser.WithDirectory("/src", source).WithDirectory("/src/internal/web/dist", runner.Directory("/src/internal/web/dist")).
			WithFile("/opt/servef", binary.File("/out/servef")).
			WithEnvVariable("SERVEF_BROWSER_BINARY", "/opt/servef").WithEnvVariable("SERVEF_BROWSER_ISOLATED", "1").
			WithEnvVariable("SERVEF_BROWSER_ARTIFACT_DIR", "/out/fixtures").WithEnvVariable("SERVEF_BROWSER_OUTPUT_DIR", "/out/playwright").
			WithEnvVariable("PORT", "18090").WithEnvVariable("SERVEF_BROWSER_WEB_PORT", "15183")
	}
	// The baseline index is private and independent of host Git metadata.
	baseline := toolchain(source).WithDirectory("/src", source).WithExec([]string{"sh", "-ec", "git init -q; git add --force --all; git -c user.name=validation -c user.email=validation@example.invalid commit -qm baseline"})
	runner = runner.WithDirectory("/src/.git", baseline.Directory("/src/.git")).WithExec([]string{"mkdir", "-p", "/out/logs"}).
		WithEnvVariable("SERVEF_CHECK_LOG_DIR", "/out/logs").WithEnvVariable("SERVEF_VALIDATION_ATTEMPT", attempt).
		WithEnvVariable("SERVEF_VALIDATION_FORCE", fmt.Sprint(force))
	if task == "" {
		task = group
	}
	command := []string{"bash", "-c", capture, "capture", "mise", "run", "--force", "--skip-deps", "_validate:" + task}
	command = append(command, args...)
	return runner.WithExec(command).Directory("/out"), nil
}

func subset(source *dagger.Directory, patterns []string) *dagger.Directory {
	return dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{Include: patterns})
}

func toolchain(source *dagger.Directory) *dagger.Container {
	c := dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From(baseImage)
	env := [][2]string{{"HOME", "/home/runner"}, {"XDG_CACHE_HOME", "/home/runner/.cache"}, {"XDG_CONFIG_HOME", "/home/runner/.config"}, {"XDG_DATA_HOME", "/home/runner/.local/share"}, {"TMPDIR", "/tmp"}, {"LANG", "C.UTF-8"}, {"TZ", "UTC"}, {"CI", "1"}, {"GOMAXPROCS", "2"}, {"GOFLAGS", "-p=2 -buildvcs=false"}, {"GOTOOLCHAIN", "local"}, {"MISE_JOBS", "2"}, {"MISE_YES", "1"}, {"MISE_TRUSTED_CONFIG_PATHS", "/src"}, {"GIT_CONFIG_GLOBAL", "/dev/null"}, {"GIT_CONFIG_NOSYSTEM", "1"}, {"GIT_AUTHOR_DATE", "2000-01-01T00:00:00Z"}, {"GIT_COMMITTER_DATE", "2000-01-01T00:00:00Z"}}
	for _, entry := range env {
		c = c.WithEnvVariable(entry[0], entry[1])
	}
	return c.WithEnvVariable("MISE_JOBS", "1").WithExec([]string{"sh", "-ec", installBase}).WithWorkdir("/src").
		WithDirectory("/src", subset(source, []string{"mise.toml", "mise.lock", ".node-version", "package.json"})).
		WithExec([]string{"mise", "install", "go", "node", "pnpm"})
}

func dependencies(source *dagger.Directory) *dagger.Container {
	return toolchain(source).WithDirectory("/src", subset(source, []string{"package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", ".npmrc", ".pnpmfile.cjs", "web/package.json", "site/package.json", "patches/**"})).
		WithExec([]string{"mise", "exec", "--", "pnpm", "install", "--frozen-lockfile"})
}

const installBase = `
set -eu
mkdir -p /home/runner /out
rm -f /etc/apt/sources.list.d/debian.sources
printf '%s\n' \
  'deb [check-valid-until=no] http://snapshot.debian.org/archive/debian/20261031T000000Z bookworm main' \
  'deb [check-valid-until=no] http://snapshot.debian.org/archive/debian/20261031T000000Z bookworm-updates main' \
  'deb [check-valid-until=no] http://snapshot.debian.org/archive/debian-security/20261031T000000Z bookworm-security main' > /etc/apt/sources.list
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends ca-certificates curl git ruby gcc libc6-dev xz-utils unzip
curl -fsSL https://github.com/jdx/mise/releases/download/v2026.10.18/mise-v2026.10.18-linux-x64.tar.xz -o /tmp/mise.tar.xz
printf '%s\n' 'f4fbb6c4d89648f789b8d909e95ae52de941b085f7a8f14bab233dabf406ef02  /tmp/mise.tar.xz' | sha256sum -c -
tar -xJf /tmp/mise.tar.xz -C /tmp
install /tmp/mise/bin/mise /usr/local/bin/mise
rm -rf /tmp/mise /tmp/mise.tar.xz /var/lib/apt/lists/*
`

const capture = `
set +e
{
  uname -sm
  cat /etc/os-release
  mise --version
  mise exec -- go version
  mise exec -- node --version
  mise exec -- pnpm --version
  mise exec -- pnpm exec playwright --version
  printf 'HOME=%s\nLANG=%s\nTZ=%s\nGOFLAGS=%s\nGOMAXPROCS=%s\nMISE_JOBS=%s\n' "$HOME" "$LANG" "$TZ" "$GOFLAGS" "$GOMAXPROCS" "$MISE_JOBS"
} > /out/environment.txt 2>&1
"$@" > /out/check.log 2>&1
status=$?
if ! git diff --exit-code --quiet HEAD; then
  printf '\nValidation changed tracked source files.\n' >> /out/check.log
  git diff HEAD >> /out/check.log
  status=1
fi
untracked=$(git ls-files --others --exclude-standard)
if [ -n "$untracked" ]; then
  printf '\nValidation created unexpected source files:\n%s\n' "$untracked" >> /out/check.log
  status=1
fi
printf '{"exitCode":%s}\n' "$status" > /out/status.json
cat /out/check.log
exit 0
`
