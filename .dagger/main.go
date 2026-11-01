// Local validation in private Linux containers with immutable preparation.
package main

import (
	"context"
	"dagger/servef-validation/internal/dagger"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type ServefValidation struct{}

const baseImage = "debian:bookworm-slim@sha256:a3bba7dc7dbcc1f37c2856c51fc9a7fac96608f50cb87b1b4e4d8b8c85ae988e"

// Run returns diagnostics and status.json even for failed checks. The caller
// must inspect exitCode before issuing a successful validation receipt.
func (m *ServefValidation) Run(ctx context.Context,
	// +ignore=[".git", "node_modules", "web/node_modules", "site/node_modules", "desktop/node_modules", "desktop/build", "desktop/test-results", "internal/web/dist", "site/dist", ".scratch", ".dev", ".env", "test-results", "playwright-report"]
	source *dagger.Directory, group string, attempt string,
	// +optional
	force bool,
	// +optional
	// +default="[]"
	argsJSON string,
	// +optional
	task string,
) (*dagger.Directory, error) {
	groups := map[string]bool{"go": true, "frontend": true, "tooling": true, "docs": true, "browser": true, "desktop": true}
	if !groups[group] || attempt == "" {
		return nil, fmt.Errorf("valid group and nonempty attempt required")
	}
	owners := map[string]string{
		"vet": "go", "test:go": "go", "test:race": "go",
		"test:web": "frontend", "test:release": "tooling",
		"build:site": "docs", "test:browser": "browser",
		"test:desktop": "desktop",
	}
	task = strings.TrimPrefix(task, "_container:")
	if task != "" && owners[task] != group {
		return nil, fmt.Errorf("task %q does not belong to group %q", task, group)
	}
	var args []string
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("args-json: %w", err)
	}
	tools, err := toolchain(ctx, source)
	if err != nil {
		return nil, err
	}
	prepared := dependencies(tools, source)
	runner := prepared.WithDirectory("/src", source)
	if group == "go" || group == "browser" || group == "desktop" {
		webSource := dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{
			Include: []string{"web/**", "patches/**", "mise.toml", "mise.lock", ".node-version", "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "site/package.json", "desktop/package.json"},
			Exclude: []string{"web/**/*.test.ts", "web/**/*.test.tsx", "web/test-fixtures/**"},
		})
		web := prepared.WithDirectory("/src", webSource)
		if force {
			web = web.WithEnvVariable("SERVEF_BUILD_ATTEMPT", attempt)
		}
		web = web.WithExec([]string{"mise", "run", "--force", "build:web"})
		runner = runner.WithDirectory("/src/internal/web/dist", web.Directory("/src/internal/web/dist"))
		modules := tools.WithDirectory("/src", subset(source, []string{"go.mod", "go.sum"})).WithExec([]string{"mise", "exec", "--", "go", "mod", "download"})
		runner = runner.WithDirectory("/home/runner/go/pkg/mod", modules.Directory("/home/runner/go/pkg/mod"))
	}
	var binary *dagger.File
	if group == "browser" || group == "desktop" {
		build := prepared.WithDirectory("/src", subset(source, []string{"mise.toml", "mise.lock", ".node-version", "*.go", "go.mod", "go.sum", "cmd/**", "internal/**"})).
			WithDirectory("/src/internal/web/dist", runner.Directory("/src/internal/web/dist")).
			WithDirectory("/home/runner/go/pkg/mod", runner.Directory("/home/runner/go/pkg/mod"))
		if force {
			build = build.WithEnvVariable("SERVEF_BUILD_ATTEMPT", attempt)
		}
		binary = build.WithExec([]string{"mise", "run", "--force", "_container:binary"}).File("/out/servef")
	}
	if group == "desktop" {
		// The app reaches the UI through the binary's daemon; it needs no dist.
		runner = desktopRuntime(prepared).WithDirectory("/src", source).
			WithFile("/opt/servef", binary).WithEnvVariable("SERVEF_DESKTOP_BINARY", "/opt/servef").
			WithEnvVariable("SERVEF_DESKTOP_OUTPUT_DIR", "/out/playwright").
			WithEnvVariable("SERVEF_DESKTOP_ELECTRON_ARGS", "--disable-dev-shm-usage --disable-gpu")
	}
	if group == "browser" {
		browser := prepared.WithExec([]string{"mise", "exec", "--", "pnpm", "exec", "playwright", "install", "--with-deps", "chromium"})
		runner = browser.WithDirectory("/src", source).WithDirectory("/src/internal/web/dist", runner.Directory("/src/internal/web/dist")).
			WithFile("/opt/servef", binary).
			WithEnvVariable("SERVEF_BROWSER_BINARY", "/opt/servef").WithEnvVariable("SERVEF_BROWSER_ISOLATED", "1").
			WithEnvVariable("SERVEF_BROWSER_ARTIFACT_DIR", "/out/fixtures").WithEnvVariable("SERVEF_BROWSER_OUTPUT_DIR", "/out/playwright").
			WithEnvVariable("PORT", "18090").WithEnvVariable("SERVEF_BROWSER_WEB_PORT", "15183")
	}
	// The baseline index is private and independent of host Git metadata.
	baseline := tools.WithDirectory("/src", source).WithExec([]string{"sh", "-ec", "git init -q; git add --force --all; git -c user.name=validation -c user.email=validation@example.invalid commit -qm baseline"})
	runner = runner.WithDirectory("/src/.git", baseline.Directory("/src/.git")).WithExec([]string{"mkdir", "-p", "/out/logs"}).
		WithEnvVariable("SERVEF_CHECK_LOG_DIR", "/out/logs").WithEnvVariable("SERVEF_VALIDATION_ATTEMPT", attempt).
		WithEnvVariable("SERVEF_VALIDATION_FORCE", fmt.Sprint(force))
	if task == "" {
		task = group
	}
	command := []string{"bash", "-c", capture, "capture", "mise", "run", "--force", "--skip-deps", "_container:" + task}
	command = append(command, args...)
	return runner.WithExec(command).Directory("/out"), nil
}

func subset(source *dagger.Directory, patterns []string) *dagger.Directory {
	return dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{Include: patterns})
}

func toolchain(ctx context.Context, source *dagger.Directory) (*dagger.Container, error) {
	lock, err := source.File("mise.lock").Contents(ctx)
	if err != nil {
		return nil, err
	}
	config, installLock, err := preparationTools(lock)
	if err != nil {
		return nil, err
	}
	c := dag.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).From(baseImage)
	env := [][2]string{{"HOME", "/home/runner"}, {"XDG_CACHE_HOME", "/home/runner/.cache"}, {"XDG_CONFIG_HOME", "/home/runner/.config"}, {"XDG_DATA_HOME", "/home/runner/.local/share"}, {"TMPDIR", "/tmp"}, {"LANG", "C.UTF-8"}, {"TZ", "UTC"}, {"CI", "1"}, {"GOMAXPROCS", "2"}, {"GOFLAGS", "-p=2 -buildvcs=false"}, {"GOTOOLCHAIN", "local"}, {"MISE_JOBS", "2"}, {"MISE_YES", "1"}, {"MISE_TRUSTED_CONFIG_PATHS", "/src"}, {"GIT_CONFIG_GLOBAL", "/dev/null"}, {"GIT_CONFIG_NOSYSTEM", "1"}, {"GIT_AUTHOR_DATE", "2000-01-01T00:00:00Z"}, {"GIT_COMMITTER_DATE", "2000-01-01T00:00:00Z"}}
	for _, entry := range env {
		c = c.WithEnvVariable(entry[0], entry[1])
	}
	return c.WithEnvVariable("MISE_JOBS", "1").WithExec([]string{"sh", "-ec", installBase}).WithWorkdir("/src").
		WithNewFile("/src/mise.toml", config).WithNewFile("/src/mise.lock", installLock).
		WithExec([]string{"mise", "install", "go", "node", "pnpm"}), nil
}

// Preparation depends on locked tools, not unrelated task definitions. Preserve
// mise's backend/checksum records while selecting exact installed versions.
func preparationTools(lock string) (string, string, error) {
	var config, selected strings.Builder
	config.WriteString("[settings]\nlockfile = true\n[tools]\n")
	selected.WriteString("lockfile_version = 3\n")
	seen := map[string]bool{}
	for _, block := range strings.Split(lock, "[[tools.")[1:] {
		name, body, ok := strings.Cut(block, "]]\n")
		if !ok || (name != "go" && name != "node" && name != "pnpm") {
			continue
		}
		if seen[name] {
			return "", "", fmt.Errorf("multiple locked versions for %s are unsupported", name)
		}
		lines := strings.Split(body, "\n")
		version := ""
		for _, line := range lines {
			if value, ok := strings.CutPrefix(line, "version = "); ok {
				var err error
				version, err = strconv.Unquote(value)
				if err != nil || version == "" {
					return "", "", fmt.Errorf("invalid locked version for %s", name)
				}
				break
			}
		}
		if version == "" {
			return "", "", fmt.Errorf("missing locked version for %s", name)
		}
		seen[name] = true
		fmt.Fprintf(&config, "%s = %q\n", name, version)
		fmt.Fprintf(&selected, "\n[[tools.%s]]\n", name)
		for _, line := range lines {
			if strings.HasPrefix(line, "specifiers = ") {
				line = fmt.Sprintf("specifiers = [%q]", version)
			}
			selected.WriteString(line + "\n")
		}
	}
	if len(seen) != 3 {
		return "", "", fmt.Errorf("mise.lock must pin go, node and pnpm")
	}
	return config.String(), selected.String(), nil
}

func dependencies(tools *dagger.Container, source *dagger.Directory) *dagger.Container {
	return tools.WithDirectory("/src", subset(source, []string{"package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", ".npmrc", ".pnpmfile.cjs", "web/package.json", "site/package.json", "desktop/package.json", "patches/**"})).
		WithExec([]string{"mise", "exec", "--", "pnpm", "install", "--frozen-lockfile"})
}

// desktopRuntime adds Xvfb, Electron's shared libraries from the pinned
// snapshot, and the checksum-verified Electron binary. ldd fails preparation
// on a missing library instead of a later opaque launch failure.
func desktopRuntime(prepared *dagger.Container) *dagger.Container {
	return prepared.WithExec([]string{"sh", "-ec", desktopPackages}).
		WithExec([]string{"mise", "exec", "--", "node", "desktop/node_modules/electron/install.js"}).
		WithExec([]string{"sh", "-ec", `electron="desktop/node_modules/electron/dist/$(cat desktop/node_modules/electron/path.txt)"; if ldd "$electron" | grep 'not found'; then exit 1; fi`})
}

const desktopPackages = `
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
  xvfb xauth fonts-dejavu-core libgtk-3-0 libnss3 libnspr4 libgbm1 libasound2 libxss1 libxtst6 \
  libcups2 libatk1.0-0 libatk-bridge2.0-0 libatspi2.0-0 libdrm2 libxkbcommon0 libxcomposite1 \
  libxdamage1 libxrandr2 libxfixes3 libpango-1.0-0 libcairo2 libdbus-1-3 libexpat1 libx11-xcb1
rm -rf /var/lib/apt/lists/*
`

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
  if [ -f desktop/node_modules/electron/package.json ]; then printf 'electron %s\n' "$(mise exec -- node -p 'require("./desktop/node_modules/electron/package.json").version')"; fi
  printf 'HOME=%s\nLANG=%s\nTZ=%s\nGOFLAGS=%s\nGOMAXPROCS=%s\nMISE_JOBS=%s\n' "$HOME" "$LANG" "$TZ" "$GOFLAGS" "$GOMAXPROCS" "$MISE_JOBS"
} > /out/environment.txt 2>&1
"$@" 2>&1 | tee /out/check.log
codes=("${PIPESTATUS[@]}")
status=${codes[0]}
if [ "$status" -eq 0 ] && [ "${codes[1]}" -ne 0 ]; then
  status=${codes[1]}
fi
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
exit 0
`
