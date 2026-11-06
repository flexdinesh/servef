// Local validation in private Linux containers with immutable preparation.
package main

import (
	"context"
	"crypto/sha256"
	"dagger/srcz-validation/internal/dagger"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type SrczValidation struct{}

const baseImage = "debian:bookworm-slim@sha256:a3bba7dc7dbcc1f37c2856c51fc9a7fac96608f50cb87b1b4e4d8b8c85ae988e"

const containerPlatform = "linux/amd64"

// aptSnapshot pins Debian packages. It names the download cache, so changing
// it starts an empty one.
const aptSnapshot = "20261031T000000Z"

// Run returns diagnostics and status.json even for failed checks. The caller
// must inspect exitCode before issuing a successful validation receipt.
func (m *SrczValidation) Run(ctx context.Context,
	// +ignore=[".git", "node_modules", "apps/web/node_modules", "apps/site/node_modules", "packages/contracts/node_modules", "apps/srcz/internal/web/dist", "apps/site/dist", ".scratch", ".dev", ".env", "test-results", "playwright-report"]
	source *dagger.Directory, group string, attempt string,
	// +optional
	force bool,
	// +optional
	// +default="[]"
	argsJSON string,
	// +optional
	task string,
) (*dagger.Directory, error) {
	groups := map[string]bool{"go": true, "frontend": true, "tooling": true, "docs": true, "browser": true, "desktop": true, "browser-extended": true, "desktop-extended": true, "webkit": true}
	if !groups[group] || attempt == "" {
		return nil, fmt.Errorf("valid group and nonempty attempt required")
	}
	owners := map[string]string{
		"vet": "go", "test:go": "go", "test:race": "go",
		"test:web": "frontend", "test:release": "tooling",
		"build:site": "docs", "test:browser": "browser",
		"test:browser:extended": "browser-extended", "test:browser:webkit": "webkit",
		"test:desktop": "desktop", "test:desktop:e2e": "desktop", "test:desktop:extended": "desktop-extended",
	}
	task = strings.TrimPrefix(task, "_container:")
	if task != "" && owners[task] != group {
		return nil, fmt.Errorf("task %q does not belong to group %q", task, group)
	}
	var args []string
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("args-json: %w", err)
	}
	tools, goCache, err := toolchain(ctx, source)
	if err != nil {
		return nil, err
	}
	prepared := dependencies(tools, source)
	runner := prepared.WithDirectory("/src", source)
	preparationGroup := group
	switch group {
	case "browser-extended", "webkit":
		preparationGroup = "browser"
	case "desktop-extended":
		preparationGroup = "desktop"
	}
	// The desktop e2e runs the real srcz binary, whose daemon serves the UI.
	desktopE2E := preparationGroup == "desktop" && task != "test:desktop"
	if preparationGroup == "go" || preparationGroup == "browser" || desktopE2E {
		webSource := dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{
			Include: []string{"apps/web/**", "packages/**", "patches/**", "mise.toml", "mise.lock", ".node-version", "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", "apps/site/package.json"},
			Exclude: []string{"apps/web/**/*.test.ts", "apps/web/**/*.test.tsx", "apps/web/test-fixtures/**"},
		})
		web := prepared.WithDirectory("/src", webSource)
		if force {
			web = web.WithEnvVariable("SRCZ_BUILD_ATTEMPT", attempt)
		}
		web = web.WithExec([]string{"mise", "run", "--force", "build:web"})
		runner = runner.WithDirectory("/src/apps/srcz/internal/web/dist", web.Directory("/src/apps/srcz/internal/web/dist"))
		// Include transitive/test dependencies in immutable preparation rather
		// than downloading them again inside each writable check container.
		modules := tools.WithDirectory("/src", subset(source, []string{"go.work", "apps/srcz/go.mod", "apps/srcz/go.sum", "tools/go.mod"})).WithExec([]string{"mise", "exec", "--", "go", "mod", "download", "all"})
		runner = runner.WithDirectory("/home/runner/go/pkg/mod", modules.Directory("/home/runner/go/pkg/mod"))
	}
	var binary *dagger.File
	if preparationGroup == "browser" || desktopE2E {
		buildSource := dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{
			Include: []string{"mise.toml", "mise.lock", ".node-version", "go.work", "tools/go.mod", "apps/srcz/go.mod", "apps/srcz/go.sum", "apps/srcz/*.go", "apps/srcz/cmd/**", "apps/srcz/internal/**", "apps/srcz/pkg/**"},
			Exclude: []string{"**/*_test.go"},
		})
		build := withGoBuildCache(prepared, goCache).WithDirectory("/src", buildSource).
			WithDirectory("/src/apps/srcz/internal/web/dist", runner.Directory("/src/apps/srcz/internal/web/dist")).
			WithDirectory("/home/runner/go/pkg/mod", runner.Directory("/home/runner/go/pkg/mod"))
		if force {
			build = build.WithEnvVariable("SRCZ_BUILD_ATTEMPT", attempt)
		}
		binary = build.WithExec([]string{"mise", "run", "--force", "_container:binary"}).File("/out/srcz")
	}
	if preparationGroup == "browser" {
		// WebKit installs only for its opt-in group; the gate prepares Chromium alone.
		engine := "chromium"
		if group == "webkit" {
			engine = "webkit"
		}
		// Playwright chooses its packages from the locked version, so this step
		// stays after dependencies; the download cache keeps a rerun cheap.
		browser := withApt(prepared, []string{"mise", "exec", "--", "pnpm", "exec", "playwright", "install", "--with-deps", engine})
		runner = browser.WithDirectory("/src", source).WithDirectory("/src/apps/srcz/internal/web/dist", runner.Directory("/src/apps/srcz/internal/web/dist")).
			WithFile("/opt/srcz", binary).
			WithEnvVariable("SRCZ_BROWSER_BINARY", "/opt/srcz").WithEnvVariable("SRCZ_BROWSER_ISOLATED", "1").
			WithEnvVariable("SRCZ_BROWSER_ARTIFACT_DIR", "/out/fixtures").WithEnvVariable("SRCZ_BROWSER_OUTPUT_DIR", "/out/playwright").
			WithEnvVariable("PORT", "18090").WithEnvVariable("SRCZ_BROWSER_WEB_PORT", "15183")
	}
	if preparationGroup == "desktop" {
		// apps/desktop is its own module on Go 1.27, outside go.work.
		desktop := tools.WithExec([]string{"sh", "-ec", desktopGo}).
			WithEnvVariable("GOWORK", "off").WithEnvVariable("SRCZ_DESKTOP_GOROOT", "/usr/local/go1.27")
		modules := desktop.WithDirectory("/src", subset(source, []string{"apps/desktop/go.mod", "apps/desktop/go.sum", "apps/srcz/go.mod", "apps/srcz/go.sum"})).
			WithEnvVariable("GOROOT", "/usr/local/go1.27").
			WithExec([]string{"/usr/local/go1.27/bin/go", "-C", "apps/desktop", "mod", "download", "all"})
		systemLibraries := ""
		if desktopE2E {
			// The built app needs a display, GTK 3, and WebKitGTK.
			systemLibraries = desktopE2EPackages
			desktop = withApt(desktop, []string{"sh", "-ec", desktopE2EPackages}).
				WithFile("/opt/srcz", binary).WithEnvVariable("SRCZ_DESKTOP_BINARY", "/opt/srcz")
		}
		runner = desktop.WithDirectory("/src", source).
			WithDirectory("/home/runner/go/pkg/mod", modules.Directory("/home/runner/go/pkg/mod"))
		goCache = goBuildCache(desktopGo, systemLibraries)
	}
	if preparationGroup == "go" || preparationGroup == "desktop" {
		runner = withGoBuildCache(runner, goCache)
	}
	if group == "browser-extended" {
		runner = runner.WithEnvVariable("SRCZ_BROWSER_EXTENDED", "1")
	}
	if group == "desktop-extended" {
		runner = runner.WithEnvVariable("SRCZ_DESKTOP_EXTENDED", "1")
	}
	// The baseline index is private and independent of host Git metadata.
	baseline := tools.WithDirectory("/src", source).WithExec([]string{"sh", "-ec", "git init -q; git add --force --all; git -c user.name=validation -c user.email=validation@example.invalid commit -qm baseline"})
	runner = runner.WithDirectory("/src/.git", baseline.Directory("/src/.git")).WithExec([]string{"mkdir", "-p", "/out/logs"}).
		WithEnvVariable("SRCZ_CHECK_LOG_DIR", "/out/logs").WithEnvVariable("SRCZ_VALIDATION_ATTEMPT", attempt).
		WithEnvVariable("SRCZ_VALIDATION_FORCE", fmt.Sprint(force))
	if task == "" {
		task = group
	}
	command := []string{"bash", "-c", capture, "capture", "mise", "run", "--force", "--skip-deps", "_container:" + task}
	command = append(command, args...)
	return runner.WithExec(command).Directory("/out"), nil
}

// withApt runs a step that installs Debian packages with one download cache
// mounted, so a retry after a mirror failure fetches only what is missing. apt
// verifies what it finds there against the snapshot's signed indexes, so the
// cache cannot change what is installed. Steps take turns (apt locks the
// directory), and the mount ends with the step, so it never reaches an image.
func withApt(c *dagger.Container, step []string) *dagger.Container {
	const archives = "/var/cache/apt/archives"
	return c.WithMountedCache(archives, dag.CacheVolume("srcz-apt-archives-"+aptSnapshot), dagger.ContainerWithMountedCacheOpts{Sharing: dagger.CacheSharingModeLocked}).
		WithExec(step).WithoutMount(archives)
}

func subset(source *dagger.Directory, patterns []string) *dagger.Directory {
	return dag.Directory().WithDirectory("/", source, dagger.DirectoryWithDirectoryOpts{Include: patterns})
}

// Only compiler outputs are mutable. Go supports concurrent access to this
// cache; source, module downloads, and prepared assets remain immutable.
func withGoBuildCache(c *dagger.Container, cache *dagger.CacheVolume) *dagger.Container {
	const path = "/home/runner/.cache/go-build"
	return c.WithEnvVariable("GOCACHE", path).
		WithMountedCache(path, cache, dagger.ContainerWithMountedCacheOpts{Sharing: dagger.CacheSharingModeShared})
}

func goBuildCache(toolchain, systemLibraries string) *dagger.CacheVolume {
	key := sha256.Sum256([]byte(strings.Join([]string{"github.com/flexdinesh/srcz", containerPlatform, toolchain, baseImage, aptSnapshot, installBase, systemLibraries}, "\x00")))
	return dag.CacheVolume(fmt.Sprintf("srcz-validation-go-build-%x", key))
}

func toolchain(ctx context.Context, source *dagger.Directory) (*dagger.Container, *dagger.CacheVolume, error) {
	lock, err := source.File("mise.lock").Contents(ctx)
	if err != nil {
		return nil, nil, err
	}
	config, installLock, err := preparationTools(lock)
	if err != nil {
		return nil, nil, err
	}
	c := dag.Container(dagger.ContainerOpts{Platform: containerPlatform}).From(baseImage)
	env := [][2]string{{"HOME", "/home/runner"}, {"XDG_CACHE_HOME", "/home/runner/.cache"}, {"XDG_CONFIG_HOME", "/home/runner/.config"}, {"XDG_DATA_HOME", "/home/runner/.local/share"}, {"TMPDIR", "/tmp"}, {"LANG", "C.UTF-8"}, {"TZ", "UTC"}, {"CI", "1"}, {"GOMAXPROCS", "2"}, {"GOFLAGS", "-p=2 -buildvcs=false"}, {"GOTOOLCHAIN", "local"}, {"MISE_JOBS", "2"}, {"MISE_YES", "1"}, {"MISE_TRUSTED_CONFIG_PATHS", "/src"}, {"GIT_CONFIG_GLOBAL", "/dev/null"}, {"GIT_CONFIG_NOSYSTEM", "1"}, {"GIT_AUTHOR_DATE", "2000-01-01T00:00:00Z"}, {"GIT_COMMITTER_DATE", "2000-01-01T00:00:00Z"}}
	for _, entry := range env {
		c = c.WithEnvVariable(entry[0], entry[1])
	}
	return withApt(c.WithEnvVariable("MISE_JOBS", "1"), []string{"sh", "-ec", installBase}).WithWorkdir("/src").
		WithNewFile("/src/mise.toml", config).WithNewFile("/src/mise.lock", installLock).
		WithExec([]string{"mise", "install", "go", "node", "pnpm"}), goBuildCache(installLock, ""), nil
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
	const store = "/home/runner/.cache/pnpm/store"
	// Copy packages into the immutable layer before removing the download store.
	return tools.WithDirectory("/src", subset(source, []string{"package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", ".npmrc", ".pnpmfile.cjs", "apps/web/package.json", "apps/site/package.json", "packages/contracts/package.json", "patches/**"})).
		WithMountedCache(store, dag.CacheVolume("srcz-validation-pnpm-store-"+containerPlatform), dagger.ContainerWithMountedCacheOpts{Sharing: dagger.CacheSharingModeShared}).
		WithExec([]string{"mise", "exec", "--", "pnpm", "install", "--frozen-lockfile", "--store-dir=" + store, "--package-import-method=copy"}).
		WithoutMount(store)
}

// desktopE2EPackages are the desktop e2e's display, session bus, GTK 3,
// WebKitGTK, Mesa, xdotool (it chooses items of native menus), and xdg-utils
// (the CLI opens srcz:// links through it).
const desktopE2EPackages = `
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
  xvfb xauth dbus dbus-x11 libgtk-3-0 libwebkit2gtk-4.1-0 libepoxy0 libegl1 libegl-mesa0 libgl1-mesa-dri libglx-mesa0 \
  xdotool xdg-utils fonts-dejavu-core
rm -rf /var/lib/apt/lists/*
`

// desktopGo installs the checksum-verified Go 1.27.3 apps/desktop requires
// (mygo v0.7.4) beside the locked toolchain.
const desktopGo = `
curl -fsSL https://go.dev/dl/go1.27.3.linux-amd64.tar.gz -o /tmp/go.tar.gz
printf '%s\n' '65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40  /tmp/go.tar.gz' | sha256sum -c -
mkdir -p /usr/local/go1.27
tar -xzf /tmp/go.tar.gz -C /usr/local/go1.27 --strip-components=1
rm /tmp/go.tar.gz
/usr/local/go1.27/bin/go version
`

// installBase also keeps downloads for the cache withApt mounts (the image's
// docker-clean deletes them after each install, which is the rest of what that
// file does) and retries the mirror's brief failures.
const installBase = `
set -eu
mkdir -p /home/runner /out
rm -f /etc/apt/sources.list.d/debian.sources /etc/apt/apt.conf.d/docker-clean
printf '%s\n' \
  'Acquire::Retries "5";' \
  'Acquire::http::Timeout "30";' \
  'Dir::Cache::pkgcache "";' \
  'Dir::Cache::srcpkgcache "";' > /etc/apt/apt.conf.d/80srcz-validation
printf '%s\n' \
  'deb [check-valid-until=no] http://snapshot.debian.org/archive/debian/` + aptSnapshot + ` bookworm main' \
  'deb [check-valid-until=no] http://snapshot.debian.org/archive/debian/` + aptSnapshot + ` bookworm-updates main' \
  'deb [check-valid-until=no] http://snapshot.debian.org/archive/debian-security/` + aptSnapshot + ` bookworm-security main' > /etc/apt/sources.list
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends ca-certificates curl git shellcheck gcc libc6-dev xz-utils unzip
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
  printf 'HOME=%s\nLANG=%s\nTZ=%s\nGOFLAGS=%s\nGOMAXPROCS=%s\nMISE_JOBS=%s\nGOCACHE=%s\n' "$HOME" "$LANG" "$TZ" "$GOFLAGS" "$GOMAXPROCS" "$MISE_JOBS" "$GOCACHE"
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
