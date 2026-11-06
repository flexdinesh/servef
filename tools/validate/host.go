//go:build linux || darwin

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type hostTools struct {
	Paths    map[string]string
	Versions map[string]string
	Files    map[string]hostToolFile
}

type hostToolFile struct {
	Target string
	Info   os.FileInfo
}

type hostProfile struct {
	Tools    hostTools
	Identity string
}

func (t hostTools) observe(name, path string) error {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	t.Files[name] = hostToolFile{target, info}
	return nil
}

// Reuse discovery and version reads within one invocation, but reject replaced,
// edited, or retargeted executables before execution and before certification.
func (t hostTools) unchanged() error {
	for name, previous := range t.Files {
		path := t.Paths[name]
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("host toolchain changed: %s: %w", name, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("host toolchain changed: %s: %w", name, err)
		}
		if target != previous.Target || !os.SameFile(previous.Info, info) || info.Size() != previous.Info.Size() || info.ModTime() != previous.Info.ModTime() || info.Mode() != previous.Info.Mode() {
			return fmt.Errorf("host toolchain changed: %s", name)
		}
	}
	return nil
}

func lockedHostVersions(entries []entry) map[string]string {
	versions := map[string]string{}
	for _, e := range entries {
		if e.Path != "mise.lock" {
			continue
		}
		section := ""
		scanner := bufio.NewScanner(strings.NewReader(string(e.Content)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "[[tools.") && strings.HasSuffix(line, "]]") {
				section = strings.TrimSuffix(strings.TrimPrefix(line, "[[tools."), "]]")
			} else if strings.HasPrefix(line, "[") {
				section = ""
			} else if strings.HasPrefix(line, "version = ") && section != "" {
				versions[section] = strings.Trim(strings.TrimPrefix(line, "version = "), "\"")
			}
		}
	}
	return versions
}

func installedHostTools(entries []entry) (hostTools, error) {
	t := hostTools{Paths: map[string]string{}, Versions: map[string]string{}, Files: map[string]hostToolFile{}}
	locked := lockedHostVersions(entries)
	for _, name := range []string{"go", "node", "pnpm"} {
		version := locked[name]
		if version == "" {
			return t, fmt.Errorf("mise.lock must pin host tool %s", name)
		}
		where, err := exec.Command("mise", "where", name+"@"+version).Output()
		if err != nil {
			return t, fmt.Errorf("install pinned host tool first: mise install %s@%s", name, version)
		}
		base := strings.TrimSpace(string(where))
		path := filepath.Join(base, "bin", name)
		if _, err := os.Stat(path); err != nil {
			path = filepath.Join(base, name)
		}
		t.Paths[name] = path
		if err := t.observe(name, path); err != nil {
			return t, err
		}
		args := []string{"--version"}
		if name == "go" {
			args = []string{"version"}
		}
		cmd := exec.Command(path, args...)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "GOTOOLCHAIN=local", "GOENV=off"}
		output, err := cmd.Output()
		if err != nil {
			return t, fmt.Errorf("read pinned %s version: %w", name, err)
		}
		actual := strings.TrimSpace(string(output))
		matches := actual == version || actual == "v"+version
		if name == "go" {
			matches = strings.HasPrefix(actual, "go version go"+version+" ")
		}
		if !matches {
			return t, fmt.Errorf("host %s version differs from mise.lock: %s", name, actual)
		}
		t.Versions[name] = actual
	}
	for _, name := range []string{"mise", "git"} {
		path, err := exec.LookPath(name)
		if err != nil {
			return t, err
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return t, err
		}
		if err := t.observe(name, path); err != nil {
			return t, err
		}
		cmd := exec.Command(path, "--version")
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent"}
		output, err := cmd.Output()
		if err != nil {
			return t, err
		}
		t.Paths[name], t.Versions[name] = path, strings.TrimSpace(string(output))
	}
	return t, t.unchanged()
}

// The status path reads versions only, without installing dependencies or
// starting an engine. Fixture-only manifests without a lock have no tool profile.
func discoverHostProfile(entries []entry, selectedGroups []string) (*hostProfile, error) {
	needed := false
	for _, group := range selectedGroups {
		needed = needed || isHostGroup(group)
	}
	if !needed {
		return nil, nil
	}
	if len(lockedHostVersions(entries)) == 0 {
		return nil, nil
	}
	t, err := installedHostTools(entries)
	if err != nil {
		return nil, err
	}
	identity := map[string]interface{}{"tools": t.Versions, "os": runtime.GOOS, "arch": runtime.GOARCH}
	if release, err := os.ReadFile("/etc/os-release"); err == nil {
		identity["os-release"] = string(release)
	} else if runtime.GOOS == "darwin" {
		release, err := exec.Command("/usr/bin/sw_vers", "-productVersion").Output()
		if err != nil {
			return nil, err
		}
		identity["os-release"] = strings.TrimSpace(string(release))
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return nil, err
	}
	return &hostProfile{Tools: t, Identity: string(raw)}, nil
}

func hostEnvironment(private, source string, tools hostTools) []string {
	paths := []string{filepath.Dir(tools.Paths["go"]), filepath.Dir(tools.Paths["node"]), filepath.Dir(tools.Paths["pnpm"]), filepath.Dir(tools.Paths["mise"]), filepath.Dir(tools.Paths["git"]), "/usr/bin", "/bin"}
	return []string{
		"PATH=" + strings.Join(paths, string(os.PathListSeparator)), "HOME=" + filepath.Join(private, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(private, "config"), "XDG_CACHE_HOME=" + filepath.Join(private, "cache"), "XDG_DATA_HOME=" + filepath.Join(private, "data"),
		"TMPDIR=" + filepath.Join(private, "tmp"), "GOPATH=" + filepath.Join(private, "go"), "GOCACHE=" + filepath.Join(private, "go-build"), "GOMODCACHE=" + filepath.Join(private, "go-mod"),
		"MISE_DATA_DIR=" + filepath.Join(private, "mise-data"), "MISE_CACHE_DIR=" + filepath.Join(private, "mise-cache"), "MISE_CONFIG_DIR=" + filepath.Join(private, "mise-config"), "MISE_STATE_DIR=" + filepath.Join(private, "mise-state"),
		"MISE_TRUSTED_CONFIG_PATHS=" + source, "MISE_YES=1", "MISE_JOBS=1", "MISE_TASK_RUN_AUTO_INSTALL=false",
		"GOTOOLCHAIN=local", "GOENV=off", "GOMAXPROCS=2", "GOFLAGS=-p=2 -buildvcs=false", "NODE_OPTIONS=--max-old-space-size=2048",
		"CI=1", "NO_COLOR=1", "LANG=C.UTF-8", "TZ=UTC", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z",
	}
}

func createHostState(private string) error {
	for _, name := range []string{"home", "config", "cache", "data", "tmp", "go", "go-build", "go-mod", "mise-data", "mise-cache", "mise-config", "mise-state"} {
		if err := os.MkdirAll(filepath.Join(private, name), 0700); err != nil {
			return err
		}
	}
	return nil
}

func (a app) executeHost(ctx context.Context, source, out, private string, m inputManifest, files []*os.File) error {
	if !isHostGroup(m.Group) {
		return fmt.Errorf("not a host group: %s", m.Group)
	}
	if err := createHostState(private); err != nil {
		return err
	}
	output := filepath.Join(out, "output")
	if err := os.MkdirAll(filepath.Join(output, "logs"), 0700); err != nil {
		return err
	}
	log, err := os.Create(filepath.Join(output, "check.log"))
	if err != nil {
		return err
	}
	defer log.Close()
	if m.host == nil || m.host.Identity != m.HostTools {
		return errors.New("host toolchain changed after input capture")
	}
	tools, identity := m.host.Tools, m.host.Identity
	if err := tools.unchanged(); err != nil {
		return err
	}
	env := append(hostEnvironment(private, source, tools), "SRCZ_CHECK_LOG_DIR="+filepath.Join(output, "logs"))
	if err := os.WriteFile(filepath.Join(output, "environment.txt"), []byte(identity+"\n"+strings.Join(env, "\n")+"\n"), 0600); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "--force", "--all"}, {"-c", "user.name=validation", "-c", "user.email=validation@example.invalid", "commit", "-qm", "baseline"}} {
		if err := commandWithFiles(ctx, source, env, log, files, tools.Paths["git"], args...); err != nil {
			return fmt.Errorf("private Git baseline: %w", err)
		}
	}
	if m.Group != "repo" {
		if err := a.hostDependencies(ctx, source, private, m, tools, log, files); err != nil {
			return err
		}
	} else if m.Task == "" || m.Task == "_host:tidy:check" || m.Task == "_host:contracts:check" {
		if err := a.hostGoModules(ctx, private, m, tools, log, files); err != nil {
			return err
		}
	}
	task := m.Task
	if task == "" {
		task = "_host:" + m.Group
	}
	args := append([]string{"run", "--skip-tools", "--force", "--skip-deps", task}, m.Args...)
	err = commandWithFiles(ctx, source, env, log, files, tools.Paths["mise"], args...)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	status := 0
	if err != nil {
		status = 1
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() > 0 {
			status = exit.ExitCode()
		}
	}
	if err := commandWithFiles(ctx, source, env, log, files, tools.Paths["git"], "diff", "--exit-code", "HEAD"); err != nil {
		fmt.Fprintln(log, "Validation changed tracked source files.")
		status = 1
	}
	cmd := exec.Command(tools.Paths["git"], "ls-files", "--others", "--exclude-standard")
	cmd.Dir, cmd.Env = source, env
	untracked, err := cmd.Output()
	if err != nil {
		return err
	}
	if len(untracked) != 0 {
		fmt.Fprintf(log, "Validation created unexpected source files:\n%s", untracked)
		status = 1
	}
	if err := tools.unchanged(); err != nil {
		return err
	}
	return writeJSON(filepath.Join(output, "status.json"), struct {
		ExitCode int `json:"exitCode"`
	}{status})
}

func (a app) hostDependencies(ctx context.Context, source, private string, m inputManifest, tools hostTools, log io.Writer, files []*os.File) error {
	var inputs []entry
	for _, e := range m.Files {
		if e.Path == "package.json" || e.Path == "pnpm-lock.yaml" || e.Path == "pnpm-workspace.yaml" || e.Path == "apps/web/package.json" || e.Path == "apps/site/package.json" || e.Path == "packages/contracts/package.json" || e.Path == ".npmrc" || e.Path == ".pnpmfile.cjs" || e.Path == "tools/validate/host.go" || strings.HasPrefix(e.Path, "patches/") {
			inputs = append(inputs, e)
		}
	}
	raw, err := json.Marshal(struct {
		Tools string
		Files []entry
	}{m.HostTools, inputs})
	if err != nil {
		return err
	}
	key := contentIdentity(raw)
	base := filepath.Join(a.Root, ".dev", "validation", "preparation")
	lock, err := acquireFile(ctx, filepath.Join(base, key+".lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	prepared := filepath.Join(base, key)
	if _, err := os.Stat(filepath.Join(prepared, ".complete")); os.IsNotExist(err) {
		stage, err := os.MkdirTemp(base, ".prepare-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		if err := materialize(stage, inputs); err != nil {
			return err
		}
		env := hostEnvironment(private, stage, tools)
		inherited := append(append([]*os.File{}, files...), lock)
		args := []string{"install", "--frozen-lockfile", "--store-dir", filepath.Join(stage, ".store"), "--package-import-method=copy"}
		if err := commandWithFiles(ctx, stage, env, log, inherited, tools.Paths["pnpm"], args...); err != nil {
			return fmt.Errorf("host dependency preparation: %w", err)
		}
		if err := os.WriteFile(filepath.Join(stage, ".complete"), []byte(key), 0600); err != nil {
			return err
		}
		if err := os.Rename(stage, prepared); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	for _, name := range []string{"node_modules", "apps/web/node_modules", "apps/site/node_modules"} {
		if err := copyPrivateTreeWithin(filepath.Join(prepared, name), filepath.Join(source, name), prepared, source); err != nil {
			return err
		}
	}
	return nil
}

func (a app) hostGoModules(ctx context.Context, private string, m inputManifest, tools hostTools, log io.Writer, files []*os.File) error {
	var inputs []entry
	for _, e := range m.Files {
		if e.Path == "go.work" || e.Path == "apps/srcz/go.mod" || e.Path == "apps/srcz/go.sum" || e.Path == "tools/go.mod" || e.Path == "tools/validate/host.go" {
			inputs = append(inputs, e)
		}
	}
	raw, err := json.Marshal(struct {
		Go, Platform string
		Files        []entry
	}{tools.Versions["go"], m.Platform, inputs})
	if err != nil {
		return err
	}
	key := "go-" + contentIdentity(raw)
	base := filepath.Join(a.Root, ".dev", "validation", "preparation")
	lock, err := acquireFile(ctx, filepath.Join(base, key+".lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	prepared := filepath.Join(base, key)
	if _, err := os.Stat(filepath.Join(prepared, ".complete")); os.IsNotExist(err) {
		stage, err := os.MkdirTemp(base, ".prepare-go-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		if err := materialize(stage, inputs); err != nil {
			return err
		}
		if err := createHostState(filepath.Join(stage, "state")); err != nil {
			return err
		}
		env := hostEnvironment(filepath.Join(stage, "state"), stage, tools)
		inherited := append(append([]*os.File{}, files...), lock)
		if err := commandWithFiles(ctx, stage, env, log, inherited, tools.Paths["go"], "mod", "download"); err != nil {
			return fmt.Errorf("host Go module preparation: %w", err)
		}
		if err := os.WriteFile(filepath.Join(stage, ".complete"), []byte(key), 0600); err != nil {
			return err
		}
		if err := os.Rename(stage, prepared); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return copyPrivateTree(filepath.Join(prepared, "state", "go-mod"), filepath.Join(private, "go-mod"))
}

// Copy bytes, never hardlinks. Package-manager symlinks remain relative to the
// copied dependency tree, and each execution may safely write its own files.
func copyPrivateTree(source, destination string) error {
	return copyPrivateTreeWithin(source, destination, source, destination)
}

func copyPrivateTreeWithin(source, destination, sourceRoot, destinationRoot string) error {
	return filepath.WalkDir(source, func(path string, item fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		info, err := item.Info()
		if err != nil {
			return err
		}
		if item.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0700)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if filepath.IsAbs(link) {
				return fmt.Errorf("absolute dependency symlink: %s", path)
			}
			for _, pair := range [][2]string{{sourceRoot, path}, {destinationRoot, target}} {
				resolved := filepath.Clean(filepath.Join(filepath.Dir(pair[1]), link))
				relative, err := filepath.Rel(pair[0], resolved)
				if err != nil || relative == ".." || strings.HasPrefix(relative, "../") {
					return fmt.Errorf("dependency symlink escapes private tree: %s", path)
				}
			}
			return os.Symlink(link, target)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported dependency entry: %s", path)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()|0600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
