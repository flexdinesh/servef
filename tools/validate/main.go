//go:build linux || darwin

// validate runs committed source through the local, explicitly managed Dagger engine.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

const engine = "servef-validation-engine-v0.21.10"
const engineImage = "registry.dagger.io/engine@sha256:2a7c054e0864e297671c24a0726ebf1c9747d70291b8843b1726daaff200a334"

type options struct {
	Group string
	Task  string
	Force bool
	Args  []string
}
type result struct {
	Group    string
	Key      string
	Status   string
	Origin   string
	ExitCode int
	Manifest inputManifest
}
type record struct {
	ID       string
	Head     string
	Tree     string
	Started  time.Time
	Finished *time.Time
	Status   string
	Results  []result
}
type receipt struct {
	Policy   string
	Key      string
	Head     string
	Run      string
	Finished time.Time
	Complete bool
}
type app struct {
	Root   string
	Cache  string
	Global string
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if ctx.Err() != nil {
			os.Exit(130)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	command := "validate"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	root, err := git(".", "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	common, err := git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(common))
	global := filepath.Join(base, "local-validation")
	a := app{root, filepath.Join(global, "servef", hex.EncodeToString(digest[:16])), global}
	if command == "prune" {
		return a.prune()
	}
	if command == "pr-create" || command == "pr-update" {
		return a.pr(ctx, command, args)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	var o options
	flags.StringVar(&o.Group, "group", "", "check group")
	flags.StringVar(&o.Task, "task", "", "specific hidden mise task")
	flags.BoolVar(&o.Force, "force", false, "execute checks and builds again")
	if err := flags.Parse(args); err != nil {
		return err
	}
	o.Args = flags.Args()
	if o.Group != "" && !slices.Contains(groups, o.Group) {
		return fmt.Errorf("unknown group %q", o.Group)
	}
	if (o.Task != "" || len(o.Args) > 0) && o.Group == "" {
		return errors.New("targeted tasks/arguments require --group")
	}
	if o.Task != "" && !taskBelongsToGroup(o.Task, o.Group) {
		return errors.New("targeted task must belong to the selected group")
	}
	switch command {
	case "validate":
		_, err = a.validate(ctx, o)
		return err
	case "status":
		return a.status(o)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func writeJSON(path string, value interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".record-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(append(raw, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
func (a app) receipt(key string) (receipt, bool) {
	var r receipt
	raw, err := os.ReadFile(filepath.Join(a.Cache, "receipts", key+".json"))
	if err != nil {
		return r, false
	}
	if json.Unmarshal(raw, &r) != nil || r.Policy != policy || r.Key != key || r.Finished.IsZero() {
		return r, false
	}
	return r, true
}
func chosen(o options) []string {
	if o.Group != "" {
		return []string{o.Group}
	}
	return groups
}

func (a app) status(o options) error {
	s, err := readSnapshot(a.Root)
	if err != nil {
		return err
	}
	complete := true
	for _, group := range chosen(o) {
		_, key, err := manifest(s, group, o.Task, o.Args)
		if err != nil {
			return err
		}
		r, ok := a.receipt(key)
		if !ok || (!r.Complete && o.Task == "" && len(o.Args) == 0) {
			fmt.Printf("missing %s %s\n", group, key)
			complete = false
		} else {
			fmt.Printf("tested %s %s (executed for %s, run %s)\n", group, key, r.Head, r.Run)
		}
	}
	if !complete {
		return errors.New("required successful validation is missing")
	}
	fmt.Printf("HEAD %s covered (%s)\n", s.Head, policy)
	return nil
}

func (a app) validate(ctx context.Context, o options) (record, error) {
	s, err := readSnapshot(a.Root)
	if err != nil {
		return record{}, err
	}
	r := record{ID: newID(), Head: s.Head, Tree: s.Tree, Started: time.Now().UTC(), Status: "running"}
	runDir := filepath.Join(a.Cache, "runs", r.ID)
	recordPath := filepath.Join(runDir, "run.json")
	if err = writeJSON(recordPath, r); err != nil {
		return r, err
	}
	finish := func(status string) {
		r.Status = status
		now := time.Now().UTC()
		r.Finished = &now
		_ = writeJSON(recordPath, r)
	}
	fmt.Printf("validation %s HEAD %s\nartifacts: %s\n", r.ID, s.Head, runDir)
	private, err := os.MkdirTemp("", "servef-validation-")
	if err != nil {
		finish("failed")
		return r, err
	}
	defer os.RemoveAll(private)
	if err = os.MkdirAll(filepath.Join(private, "client-home"), 0700); err != nil {
		finish("failed")
		return r, err
	}
	moduleDir := filepath.Join(private, "module")
	var moduleFiles []entry
	for _, e := range s.Entries {
		if e.Path == "dagger.json" || strings.HasPrefix(e.Path, ".dagger/") {
			moduleFiles = append(moduleFiles, e)
		}
	}
	if err = materialize(moduleDir, moduleFiles); err != nil {
		finish("failed")
		return r, err
	}
	var slot *os.File
	var daggerBinary string
	defer func() {
		if slot != nil {
			slot.Close()
		}
	}()
	for _, group := range chosen(o) {
		m, key, err := manifest(s, group, o.Task, o.Args)
		if err != nil {
			finish("failed")
			return r, err
		}
		value := result{Group: group, Key: key, Manifest: m}
		if cached, ok := a.receipt(key); ok && !o.Force {
			value.Status = "reused"
			value.Origin = cached.Run
			r.Results = append(r.Results, value)
			fmt.Printf("reuse %s %s\n", group, key)
			_ = writeJSON(recordPath, r)
			continue
		}
		if slot == nil {
			if err = checkEngine(); err != nil {
				finish("failed")
				return r, err
			}
			daggerBinary, err = resolveDagger(a.Root)
			if err != nil {
				finish("failed")
				return r, err
			}
			slot, err = acquireSlot(ctx, a.Global)
			if err != nil {
				finish("cancelled")
				return r, err
			}
		}
		if ctx.Err() != nil {
			finish("cancelled")
			return r, ctx.Err()
		}
		source := filepath.Join(private, group)
		if err = materialize(source, m.Files); err != nil {
			finish("failed")
			return r, err
		}
		out := filepath.Join(runDir, group)
		if err = os.MkdirAll(out, 0700); err != nil {
			finish("failed")
			return r, err
		}
		log, err := os.Create(filepath.Join(out, "dagger.log"))
		if err != nil {
			finish("failed")
			return r, err
		}
		arguments, _ := json.Marshal(o.Args)
		if o.Args == nil {
			arguments = []byte("[]")
		}
		cmdArgs := []string{"-m", moduleDir, "call", "run", "--source", source, "--group", group, "--attempt", newID(), fmt.Sprintf("--force=%t", o.Force), "--args-json", string(arguments), "--task", o.Task, "export", "--path", filepath.Join(out, "output")}
		err = commandWithFiles(ctx, moduleDir, runnerEnvironment(filepath.Join(private, "client-home")), log, []*os.File{slot}, daggerBinary, cmdArgs...)
		log.Close()
		value.Status = "executed"
		value.Origin = r.ID
		var state struct {
			ExitCode *int `json:"exitCode"`
		}
		raw, readErr := os.ReadFile(filepath.Join(out, "output", "status.json"))
		if err == nil {
			if readErr != nil {
				err = readErr
			} else if e := json.Unmarshal(raw, &state); e != nil {
				err = e
			} else if state.ExitCode == nil {
				err = errors.New("missing check exit status")
			} else {
				value.ExitCode = *state.ExitCode
				if value.ExitCode != 0 {
					err = fmt.Errorf("%s failed with status %d", group, value.ExitCode)
				}
			}
		}
		if err != nil {
			value.Status = "failed"
			r.Results = append(r.Results, value)
			status := "failed"
			if ctx.Err() != nil {
				status = "cancelled"
			}
			finish(status)
			return r, fmt.Errorf("%w; log: %s", err, filepath.Join(out, "dagger.log"))
		}
		r.Results = append(r.Results, value)
		proof := receipt{policy, key, s.Head, r.ID, time.Now().UTC(), o.Task == "" && len(o.Args) == 0}
		if err = writeJSON(filepath.Join(a.Cache, "receipts", key+".json"), proof); err != nil {
			finish("failed")
			return r, err
		}
		fmt.Printf("ok %s %s\n", group, key)
		_ = writeJSON(recordPath, r)
	}
	finish("success")
	return r, nil
}

func runnerEnvironment(home string) []string {
	// Only host tooling discovery passes through. No Dagger Cloud credentials,
	// Docker contexts, application URLs, Git hooks, or personal mise overrides.
	env := []string{"_EXPERIMENTAL_DAGGER_RUNNER_HOST=docker-container://" + engine, "DAGGER_LEAVE_OLD_ENGINE=true", "NO_COLOR=1", "HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "XDG_DATA_HOME=" + filepath.Join(home, "data")}
	for _, key := range []string{"PATH", "USER"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func command(ctx context.Context, dir string, env []string, out io.Writer, name string, args ...string) error {
	return commandWithFiles(ctx, dir, env, out, nil, name, args...)
}

func commandWithFiles(ctx context.Context, dir string, env []string, out io.Writer, files []*os.File, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.ExtraFiles = files
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return ctx.Err()
	}
}

func acquireSlot(ctx context.Context, dir string) (*os.File, error) {
	dir = filepath.Join(dir, "slots")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	announced := false
	for {
		for i := range 2 {
			f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%d.lock", i)), os.O_CREATE|os.O_RDWR, 0600)
			if err != nil {
				return nil, err
			}
			err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			if err == nil {
				return f, nil
			}
			f.Close()
			if !errors.Is(err, syscall.EWOULDBLOCK) {
				return nil, err
			}
		}
		if !announced {
			fmt.Println("waiting for local validation capacity")
			announced = true
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a app) prune() error {
	entries, err := os.ReadDir(filepath.Join(a.Cache, "runs"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(a.Cache, "runs", entry.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "run.json"))
		if err != nil {
			continue
		}
		var r record
		if json.Unmarshal(raw, &r) != nil || r.Finished == nil || !r.Finished.Before(cutoff) {
			continue
		}
		if err = os.RemoveAll(dir); err != nil {
			return err
		}
		fmt.Println("pruned", r.ID)
	}
	return nil
}

func (a app) pr(ctx context.Context, operation string, args []string) error {
	if operation == "pr-update" && len(args) > 0 {
		return errors.New("pr-update accepts no arguments")
	}
	forwarded, err := prArguments(args)
	if err != nil {
		return err
	}
	r, err := a.validate(ctx, options{})
	if err != nil {
		return err
	}
	s, err := readSnapshot(a.Root)
	if err != nil {
		return err
	}
	if s.Head != r.Head {
		return errors.New("HEAD changed during validation; refusing PR operation")
	}
	branch, err := git(a.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return errors.New("PR operation requires a branch")
	}
	if branch == "main" {
		return errors.New("PR operation requires a feature branch")
	}
	if err = command(ctx, a.Root, gitEnvironment(), os.Stdout, "git", "push", "origin", r.Head+":refs/heads/"+branch); err != nil {
		return err
	}
	remote, err := git(a.Root, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil {
		return err
	}
	if len(strings.Fields(remote)) != 2 || strings.Fields(remote)[0] != r.Head {
		return errors.New("remote HEAD does not match validated commit")
	}
	current, err := readSnapshot(a.Root)
	if err != nil {
		return err
	}
	if current.Head != r.Head {
		return errors.New("HEAD changed after push; refusing PR creation")
	}
	if operation == "pr-update" {
		return nil
	}
	origin, err := git(a.Root, "remote", "get-url", "origin")
	if err != nil {
		return err
	}
	create := []string{"pr", "create", "--repo", origin, "--base", "main", "--head", branch}
	create = append(create, forwarded...)
	return command(ctx, a.Root, os.Environ(), os.Stdout, "gh", create...)
}

func taskBelongsToGroup(task, group string) bool {
	owners := map[string]string{
		"fmt:check": "go", "tidy:check": "go", "vet": "go", "test:go": "go", "test:race": "go",
		"test:web": "frontend", "typecheck:web": "frontend", "lint": "frontend",
		"test:release": "tooling", "typecheck:root": "tooling", "mise:check": "tooling",
		"typecheck:site": "docs", "build:site": "docs", "test:browser": "browser",
	}
	name, ok := strings.CutPrefix(task, "_validate:")
	return ok && group != "" && owners[name] == group
}

// Parse values as values, never as flags. An allowlist also rejects attached
// gh target shorthands such as -Hother and -Rowner/repository before any push.
func prArguments(args []string) ([]string, error) {
	values := map[string]bool{"--title": true, "-t": true, "--body": true, "-b": true, "--body-file": true, "-F": true, "--reviewer": true, "-r": true, "--assignee": true, "-a": true, "--label": true, "-l": true, "--milestone": true, "-m": true, "--project": true, "-p": true}
	switches := map[string]bool{"--fill": true, "--fill-first": true, "--fill-verbose": true, "--draft": true, "-d": true, "--no-maintainer-edit": true}
	var out []string
	ready, draft := false, false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--ready" {
			ready = true
			continue
		}
		name, value, equals := strings.Cut(arg, "=")
		if values[name] {
			if !equals {
				i++
				if i == len(args) {
					return nil, fmt.Errorf("%s requires a value", name)
				}
				value = args[i]
			}
			out = append(out, name, value)
			continue
		}
		if !equals && switches[arg] {
			draft = draft || arg == "--draft" || arg == "-d"
			out = append(out, arg)
			continue
		}
		return nil, fmt.Errorf("unsupported PR argument %q; repository/head/base are fixed", arg)
	}
	if ready && draft {
		return nil, errors.New("choose --ready or --draft")
	}
	if !ready && !draft {
		out = append(out, "--draft")
	}
	return out, nil
}

func resolveDagger(root string) (string, error) {
	cmd := exec.Command("mise", "where", "dagger@0.21.10")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New("install pinned CLI first: mise install dagger@0.21.10")
	}
	binary := filepath.Join(strings.TrimSpace(string(out)), "dagger")
	if info, err := os.Stat(binary); err != nil || info.IsDir() {
		return "", fmt.Errorf("pinned Dagger binary unavailable: %s", binary)
	}
	return binary, nil
}

func checkEngine() error {
	cmd := exec.Command("docker", "inspect", engine, "--format", `{{.State.Running}} {{.HostConfig.NanoCpus}} {{.HostConfig.Memory}} {{.Config.Image}}`)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("managed engine %s unavailable; follow docs/validation.md engine setup; automatic bootstrap is disabled", engine)
	}
	var running bool
	var cpus, memory int64
	var image string
	if _, err = fmt.Sscan(string(out), &running, &cpus, &memory, &image); err != nil {
		return err
	}
	if image != engineImage {
		return errors.New("managed engine image differs from the validation pin; see docs/validation.md")
	}
	if !running {
		return errors.New("managed validation engine is stopped; start it explicitly")
	}
	if cpus < 2_000_000_000 || memory < 4*1024*1024*1024 {
		return errors.New("managed engine requires explicit limits of at least 2 CPUs and 4 GiB; see docs/validation.md")
	}
	return nil
}
