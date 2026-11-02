package cmd

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/internal/control"
)

// serveRegistered starts a server on 127.0.0.1 whose control endpoint
// answers handlerRegistry's token, and registers it in registry.
func serveRegistered(t *testing.T, registry, handlerRegistry control.Registry) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	instance, err := handlerRegistry.Register("127.0.0.1", port)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	if handlerRegistry != registry {
		other, err := registry.Register("127.0.0.1", port)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = other.Close() })
	}
	server := &http.Server{Handler: instance.Handler(http.NotFoundHandler())}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return port
}

func TestControlListPrintsOnlyRunningServers(t *testing.T) {
	directory := t.TempDir()
	registry := control.NewRegistry(directory)
	running := serveRegistered(t, registry, registry)
	// A server answering another token is not the registered process.
	serveRegistered(t, registry, control.NewRegistry(t.TempDir()))
	// A stale record: nothing listens on its port.
	stale, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stalePort := stale.Addr().(*net.TCPAddr).Port
	_ = stale.Close()
	if _, err := registry.Register("127.0.0.1", stalePort); err != nil {
		t.Fatal(err)
	}

	execute := func(args ...string) string {
		t.Helper()
		command := newControlCommand(directory)
		var stdout bytes.Buffer
		command.SetOut(&stdout)
		command.SetErr(&stdout)
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return stdout.String()
	}

	origin := "http://127.0.0.1:" + strconv.Itoa(running)
	var listed struct {
		Servers []map[string]any `json:"servers"`
	}
	output := execute("list", "--json")
	if err := json.Unmarshal([]byte(output), &listed); err != nil {
		t.Fatalf("JSON %q: %v", output, err)
	}
	want := map[string]any{"host": "127.0.0.1", "port": float64(running), "origin": origin}
	if len(listed.Servers) != 1 || len(listed.Servers[0]) != len(want) {
		t.Fatalf("servers = %v, want [%v]", listed.Servers, want)
	}
	for key, value := range want {
		if listed.Servers[0][key] != value {
			t.Errorf("%s = %v, want %v", key, listed.Servers[0][key], value)
		}
	}
	if strings.Contains(output, "token") {
		t.Errorf("output %q carries a token", output)
	}
	if plain := execute("list"); plain != origin+"\n" {
		t.Errorf("plain = %q, want %q", plain, origin+"\n")
	}
}

func TestControlListWithNoServersPrintsAnEmptyList(t *testing.T) {
	var stdout bytes.Buffer
	if err := listServers(t.Context(), control.NewRegistry(t.TempDir()), true, &stdout); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "{\"servers\":[]}\n" {
		t.Errorf("output = %q", got)
	}
}

func TestRootCommandHidesControlPlumbing(t *testing.T) {
	command, _, err := newRootCommandWith(nil, nil).Find([]string{"control", "list"})
	if err != nil || command.Name() != "list" || !command.Parent().Hidden {
		t.Fatalf("control list = %v, %v; want a hidden control command", command, err)
	}
}
