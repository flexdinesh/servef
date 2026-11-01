//go:build linux || darwin

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This opt-in test exercises the actual engine, not a mocked command runner.
func TestDaggerIsolationAcceptance(t *testing.T) {
	if os.Getenv("SERVEF_DAGGER_ACCEPTANCE") != "1" {
		t.Skip("set SERVEF_DAGGER_ACCEPTANCE=1 with the explicitly managed engine running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	where, err := exec.Command("mise", "where", "dagger@0.21.10").Output()
	if err != nil {
		t.Fatal(err)
	}
	dagger := filepath.Join(strings.TrimSpace(string(where)), "bin", "dagger")
	if _, err := os.Stat(dagger); err != nil {
		dagger = filepath.Join(strings.TrimSpace(string(where)), "dagger")
	}
	root := t.TempDir()
	source := filepath.Join(root, "probe.go")
	if err := os.WriteFile(source, []byte(isolationProbe), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "probe")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, source)
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build probe: %v\n%s", err, output)
	}
	engineIDs := func() string {
		out, err := exec.Command("docker", "ps", "--filter", "name=engine", "--format", "{{.Names}} {{.ID}}").Output()
		if err != nil {
			t.Fatal("inspect engine identities: ", err)
		}
		return string(out)
	}
	before := engineIDs()
	env := runnerEnvironment(filepath.Join(root, "home"))
	for _, dir := range []string{"home", "home/.cache", "home/.config", "home/.local/share"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	args := func(mode, marker, duration string) []string {
		argv := strings.Join([]string{"/probe", mode, marker, duration}, ",")
		return []string{"--progress=plain", "core", "container", "from", "--address", "debian:bookworm-slim@sha256:a3bba7dc7dbcc1f37c2856c51fc9a7fac96608f50cb87b1b4e4d8b8c85ae988e", "with-file", "--path", "/probe", "--source", binary, "with-exec", "--args", argv, "stdout"}
	}
	var hostRequests atomic.Int32
	listener, err := net.Listen("tcp", "127.0.0.1:18090")
	if err != nil {
		t.Fatal("host port 18090 must be free for acceptance: ", err)
	}
	host := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hostRequests.Add(1)
		fmt.Fprint(w, "host-must-not-satisfy-runner")
	})}
	go host.Serve(listener)
	defer host.Close()
	missing := &acceptanceOutput{}
	if err := command(ctx, root, env, missing, dagger, args("missing", newID(), "0")...); err != nil {
		t.Fatalf("host isolation: %v\n%s", err, missing.String())
	}
	if hostRequests.Load() != 0 || !strings.Contains(missing.String(), "ISOLATED") {
		t.Fatalf("host answered runner request: %s", missing.String())
	}
	firstID, secondID := newID(), newID()
	first := &acceptanceOutput{ready: make(chan struct{}), marker: "READY:" + firstID}
	second := &acceptanceOutput{ready: make(chan struct{}), marker: "READY:" + secondID}
	firstCtx, stopFirst := context.WithCancel(ctx)
	defer stopFirst()
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	go func() { firstDone <- command(firstCtx, root, env, first, dagger, args("serve", firstID, "90")...) }()
	go func() { secondDone <- command(ctx, root, env, second, dagger, args("serve", secondID, "5")...) }()
	for _, output := range []*acceptanceOutput{first, second} {
		select {
		case <-output.ready:
		case <-ctx.Done():
			t.Fatalf("server never ready: %s", output.String())
		}
	}
	stopFirst()
	if err := <-firstDone; err == nil {
		t.Fatal("interrupted Dagger invocation succeeded")
	}
	if err := <-secondDone; err != nil || !strings.Contains(second.String(), "RESULT:"+secondID) {
		t.Fatalf("other invocation affected: %v\n%s", err, second.String())
	}
	restarted := &acceptanceOutput{}
	if err := command(ctx, root, env, restarted, dagger, args("serve", newID(), "0")...); err != nil {
		t.Fatalf("restart after cancellation failed: %v\n%s", err, restarted.String())
	}
	if engineIDs() != before {
		t.Fatal("engine identities changed")
	}
	if hostRequests.Load() != 0 {
		t.Fatal("host received runner-local request")
	}
	failureMarker := newID()
	failed := &acceptanceOutput{}
	failedArgs := args("fail", failureMarker, "0")
	failedArgs = append(failedArgs[:len(failedArgs)-1], "--expect", "ANY", "directory", "--path", "/out", "export", "--path", filepath.Join(root, "failed-artifacts"))
	if err := command(ctx, root, env, failed, dagger, failedArgs...); err != nil {
		t.Fatalf("export failure artifact: %v\n%s", err, failed.String())
	}
	diagnostic, err := os.ReadFile(filepath.Join(root, "failed-artifacts", "failure.log"))
	if err != nil || string(diagnostic) != failureMarker {
		t.Fatalf("failed execution diagnostics lost: %q %v", diagnostic, err)
	}
	t.Log("Real Dagger: identical internal ports, distinct markers, host exclusion, independent cancellation, recovery, engine preservation, failed-exec artifact export passed")
}

type acceptanceOutput struct {
	mu     sync.Mutex
	text   strings.Builder
	ready  chan struct{}
	marker string
	once   sync.Once
}

func (o *acceptanceOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n, err := o.text.Write(p)
	if o.ready != nil && strings.Contains(o.text.String(), o.marker) {
		o.once.Do(func() { close(o.ready) })
	}
	return n, err
}

func (o *acceptanceOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.text.String()
}

const isolationProbe = `package main
import("fmt";"io";"net";"net/http";"os";"strconv";"time")
func main(){
 client:=http.Client{Timeout:2*time.Second}
 if os.Args[1]=="fail" {if e:=os.MkdirAll("/out",0700);e!=nil{panic(e)};if e:=os.WriteFile("/out/failure.log",[]byte(os.Args[2]),0600);e!=nil{panic(e)};os.Exit(7)}
 if os.Args[1]=="missing" { r,e:=client.Get("http://127.0.0.1:18090"); if e==nil {r.Body.Close(); panic("host response crossed isolation")}; fmt.Println("ISOLATED");return }
 marker:=os.Args[2]
 l,e:=net.Listen("tcp","127.0.0.1:18090");if e!=nil {panic(e)}
 s:=http.Server{Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){fmt.Fprint(w,marker)})}
 go s.Serve(l)
 r,e:=client.Get("http://127.0.0.1:18090");if e!=nil {panic(e)};b,e:=io.ReadAll(r.Body);r.Body.Close();if e!=nil||string(b)!=marker {panic("wrong private server")}
 fmt.Println("READY:"+marker)
 seconds,_:=strconv.Atoi(os.Args[3]);time.Sleep(time.Duration(seconds)*time.Second)
 fmt.Println("RESULT:"+marker);s.Close()
}`
