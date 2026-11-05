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
	"strings"
	"syscall"
	"time"
)

const engine = "servef-validation-engine-v0.21.10"
const engineImage = "registry.dagger.io/engine@sha256:2a7c054e0864e297671c24a0726ebf1c9747d70291b8843b1726daaff200a334"

type options struct {
	Suite   string
	Group   string
	Task    string
	Force   bool
	Working bool
	Args    []string
}
type result struct {
	Group      string
	Key        string
	Status     string
	Origin     string
	ExitCode   int
	DurationMS int64
	Manifest   inputManifest
}
type record struct {
	ID        string
	Head      string
	Tree      string
	Started   time.Time
	Finished  *time.Time
	Status    string
	Committed bool
	Artifacts string
	Results   []result
}
type receipt struct {
	Policy      string
	Key         string
	Head        string
	Run         string
	Finished    time.Time
	Complete    bool
	Environment string
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
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	var o options
	o.Working = command == "working"
	flags.StringVar(&o.Suite, "suite", "correctness", "check suite: correctness, extended, full")
	flags.StringVar(&o.Group, "group", "", "check group")
	flags.StringVar(&o.Task, "task", "", "specific hidden mise task")
	flags.BoolVar(&o.Force, "force", false, "execute checks and builds again")
	if err := flags.Parse(args); err != nil {
		return err
	}
	o.Args = flags.Args()
	if !knownSuite(o.Suite) {
		return fmt.Errorf("unknown suite %q", o.Suite)
	}
	if o.Group != "" && !knownGroup(o.Group) {
		return fmt.Errorf("unknown group %q", o.Group)
	}
	if (o.Task != "" || len(o.Args) > 0) && o.Group == "" {
		return errors.New("targeted tasks/arguments require --group")
	}
	if o.Task != "" && !taskBelongsToGroup(o.Task, o.Group) {
		return errors.New("targeted task must belong to the selected group")
	}
	switch command {
	case "validate", "working":
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
	if _, err := os.Stat(filepath.Join(a.Cache, "pending", key+".json")); !errors.Is(err, os.ErrNotExist) {
		return r, false
	}
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
	switch o.Suite {
	case "extended":
		return extendedGroups
	case "full":
		return append(append([]string{}, groups...), extendedGroups...)
	default:
		return groups
	}
}

func (a app) status(o options) error {
	s, err := readSnapshot(a.Root)
	if err != nil {
		return err
	}
	host, err := discoverHostProfile(s.Entries, chosen(o))
	if err != nil {
		return err
	}
	complete := true
	var locks []*os.File
	defer func() {
		for _, f := range locks {
			f.Close()
		}
	}()
	for _, group := range chosen(o) {
		_, key, err := manifest(s, group, o.Task, o.Args, host)
		if err != nil {
			return err
		}
		lock, err := tryReadCertificateLock(filepath.Join(a.Cache, "locks", key+".lock"))
		if err != nil {
			return fmt.Errorf("%s certification is busy or unavailable: %w", group, err)
		}
		locks = append(locks, lock)
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
	if host != nil {
		if err := host.Tools.unchanged(); err != nil {
			return err
		}
	}
	if err := unchangedSnapshot(a.Root, s); err != nil {
		return err
	}
	fmt.Printf("HEAD %s covered (%s)\n", s.Head, policy)
	return nil
}

func (a app) validate(ctx context.Context, o options) (record, error) {
	var s snapshot
	var err error
	if o.Working {
		s, err = readWorkingSnapshot(a.Root)
	} else {
		s, err = readSnapshot(a.Root)
	}
	if err != nil {
		return record{}, err
	}
	if err = ctx.Err(); err != nil {
		return record{}, err
	}
	host, err := discoverHostProfile(s.Entries, chosen(o))
	if err != nil {
		return record{}, err
	}
	r := record{ID: newID(), Head: s.Head, Tree: s.Tree, Started: time.Now().UTC(), Status: "running", Committed: !o.Working}
	runDir := filepath.Join(a.artifactRoot(), r.ID)
	r.Artifacts = runDir
	lease, err := acquireFile(ctx, filepath.Join(runDir, "active.lock"))
	if err != nil {
		return r, err
	}
	defer lease.Close()
	recordPath := filepath.Join(runDir, "run.json")
	if err = writeJSON(recordPath, r); err != nil {
		return r, err
	}
	finish := func(status string) error {
		r.Status = status
		now := time.Now().UTC()
		r.Finished = &now
		return writeJSON(recordPath, r)
	}
	fail := func(err error) (record, error) {
		status := "failed"
		if ctx.Err() != nil {
			status = "cancelled"
		}
		if saveErr := finish(status); saveErr != nil {
			return r, errors.Join(err, saveErr)
		}
		return r, err
	}
	fmt.Printf("validation %s HEAD %s committed=%t\nartifacts: %s\n", r.ID, s.Head, r.Committed, runDir)
	private := filepath.Join(runDir, "private")
	if err = os.MkdirAll(private, 0700); err != nil {
		return fail(err)
	}
	defer os.RemoveAll(private)
	moduleDir := filepath.Join(private, "module")
	var moduleFiles []entry
	for _, e := range s.Entries {
		if e.Path == "dagger.json" || strings.HasPrefix(e.Path, ".dagger/") {
			moduleFiles = append(moduleFiles, e)
		}
	}
	if err = materialize(moduleDir, moduleFiles); err != nil {
		return fail(err)
	}
	// Worktree-local snapshots live under ignored .dev. Give Dagger its own
	// repository boundary so it cannot resolve the parent workspace/ignore rules.
	if _, err = git(moduleDir, "init", "-q"); err != nil {
		return fail(err)
	}
	for _, group := range chosen(o) {
		if err = ctx.Err(); err != nil {
			return fail(err)
		}
		m, key, err := manifest(s, group, o.Task, o.Args, host)
		if err != nil {
			return fail(err)
		}
		value, err := a.checkGroup(ctx, o, s, m, key, runDir, private, moduleDir, lease)
		r.Results = append(r.Results, value)
		if err != nil {
			return fail(err)
		}
		if err = writeJSON(recordPath, r); err != nil {
			return fail(err)
		}
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	if !o.Working {
		if err = unchangedSnapshot(a.Root, s); err != nil {
			return fail(err)
		}
	}
	if host != nil {
		if err = host.Tools.unchanged(); err != nil {
			return fail(err)
		}
	}
	if err = finish("success"); err != nil {
		return r, err
	}
	return r, nil
}

func unchangedSnapshot(root string, original snapshot) error {
	current, err := readSnapshot(root)
	if err != nil {
		return err
	}
	if current.Head != original.Head || current.Tree != original.Tree {
		return errors.New("HEAD changed during validation; no aggregate certification")
	}
	return nil
}

func (a app) checkGroup(ctx context.Context, o options, s snapshot, m inputManifest, key, runDir, private, moduleDir string, lease *os.File) (result, error) {
	v := result{Group: m.Group, Key: key, Manifest: m, Origin: runDir}
	// Editing runs never read or publish committed certificates. They remain
	// independent executions even when their source happens to match HEAD.
	var keyLock *os.File
	var err error
	if !o.Working {
		keyLock, err = acquireFile(ctx, filepath.Join(a.Cache, "locks", key+".lock"))
		if err != nil {
			return v, err
		}
		defer keyLock.Close()
		if cached, ok := a.receipt(key); ok && !o.Force && (cached.Complete || o.Task != "" || len(o.Args) > 0) {
			if err = ctx.Err(); err != nil {
				return v, err
			}
			v.Status = "reused"
			v.Origin = cached.Run
			fmt.Printf("reuse %s %s\n", m.Group, key)
			return v, nil
		}
	}
	if !isHostGroup(m.Group) {
		if err = checkEngine(); err != nil {
			return v, err
		}
	}
	slot, err := acquireSlot(ctx, a.Global)
	if err != nil {
		return v, err
	}
	defer slot.Close()
	if err = ctx.Err(); err != nil {
		return v, err
	}
	if !o.Working {
		if err = a.invalidate(key, runDir); err != nil {
			return v, err
		}
	}
	v.Status = "failed"
	source := filepath.Join(private, m.Group)
	if err = materialize(source, m.Files); err != nil {
		return v, err
	}
	out := filepath.Join(runDir, m.Group)
	if err = os.MkdirAll(out, 0700); err != nil {
		return v, err
	}
	files := []*os.File{lease, slot}
	if keyLock != nil {
		files = append(files, keyLock)
	}
	started := time.Now()
	if isHostGroup(m.Group) {
		err = a.executeHost(ctx, source, out, filepath.Join(private, "host-"+m.Group), m, files)
	} else {
		err = a.executeDagger(ctx, o, source, out, private, moduleDir, m.Group, files)
	}
	v.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		return v, fmt.Errorf("%w; artifacts: %s", err, out)
	}
	var state struct {
		ExitCode *int `json:"exitCode"`
	}
	raw, err := os.ReadFile(filepath.Join(out, "output", "status.json"))
	if err != nil {
		return v, err
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return v, err
	}
	if state.ExitCode == nil {
		return v, errors.New("missing check exit status")
	}
	v.ExitCode = *state.ExitCode
	if v.ExitCode != 0 {
		return v, fmt.Errorf("%s failed with status %d; artifacts: %s", m.Group, v.ExitCode, out)
	}
	environment, err := os.ReadFile(filepath.Join(out, "output", "environment.txt"))
	if err != nil {
		return v, fmt.Errorf("missing environment evidence: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return v, err
	}
	if !o.Working {
		proof := receipt{Policy: policy, Key: key, Head: s.Head, Run: runDir, Finished: time.Now().UTC(), Complete: o.Task == "" && len(o.Args) == 0, Environment: string(environment)}
		if err = a.certify(proof); err != nil {
			return v, err
		}
	}
	v.Status = "executed"
	fmt.Printf("ok %s %s\n", m.Group, key)
	return v, nil
}

func (a app) executeDagger(ctx context.Context, o options, source, out, private, moduleDir, group string, files []*os.File) error {
	if err := os.MkdirAll(filepath.Join(private, "client-home"), 0700); err != nil {
		return err
	}
	binary, err := resolveDagger(a.Root)
	if err != nil {
		return err
	}
	log, err := os.Create(filepath.Join(out, "dagger.log"))
	if err != nil {
		return err
	}
	defer log.Close()
	args, _ := json.Marshal(o.Args)
	if len(o.Args) == 0 {
		args = []byte("[]")
	}
	argv := []string{"-m", moduleDir, "call", "run", "--source", source, "--group", group, "--attempt", newID(), fmt.Sprintf("--force=%t", o.Force), "--args-json", string(args), "--task", o.Task, "export", "--path", filepath.Join(out, "output")}
	return commandWithFiles(ctx, moduleDir, runnerEnvironment(filepath.Join(private, "client-home")), log, files, binary, argv...)
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
