package link

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want Target
	}{
		{"servef-native://local/?project=%2Fhome%2Fdee%2Fcode&path=README.md", Target{Kind: Local, Project: "/home/dee/code", Path: "README.md"}},
		{"servef-native://local", Target{Kind: Local}},
		{"servef-native://local/view?project=%2Fp&review=working", Target{Kind: Local, Project: "/p", Review: true}},
		{"servef-native://LocalHost:7971/?project=%2Fp", Target{Kind: Server, Host: "localhost", Port: 7971, Project: "/p"}},
		{"servef-native://10.0.0.5:7975/view?path=a.go", Target{Kind: Server, Host: "10.0.0.5", Port: 7975, Path: "a.go"}},
		{"servef-native://[::1]:7971/", Target{Kind: Server, Host: "::1", Port: 7971}},
	} {
		got, err := Parse(tc.raw)
		if err != nil || got != tc.want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", tc.raw, got, err, tc.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, raw := range []string{
		"servef://local/?project=/p",           // another scheme
		"servef-native://local/settings",       // unknown route
		"servef-native://example.com/",         // no port
		"servef-native://local:7971/",          // local names the daemon only
		"servef-native://user@localhost:7971/", // credentials
		"servef-native://localhost:0/",
		"servef-native://localhost:65536/",
		"servef-native://localhost:07971/",
		"servef-native:local",
		"http://localhost:7971/",
	} {
		if _, err := Parse(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) = %v, want ErrInvalid", raw, err)
		}
	}
}

func TestOrigin(t *testing.T) {
	if got := (Target{Kind: Server, Host: "::1", Port: 7971}).Origin(); got != "http://[::1]:7971" {
		t.Fatalf("Origin = %q", got)
	}
}

func TestParseServer(t *testing.T) {
	for raw, want := range map[string]Target{
		"localhost:7971": {Kind: Server, Host: "localhost", Port: 7971},
		" http://127.0.0.1:7972/view?project=%2Fp ": {Kind: Server, Host: "127.0.0.1", Port: 7972, Project: "/p"},
		"servef://box.lan:7975/":                    {Kind: Server, Host: "box.lan", Port: 7975},
		"servef-native://localhost:7971/?path=a":    {Kind: Server, Host: "localhost", Port: 7971, Path: "a"},
	} {
		got, err := ParseServer(raw)
		if err != nil || got != want {
			t.Errorf("ParseServer(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "https://localhost:7971", "localhost", "servef://local/", "ftp://x:1"} {
		if _, err := ParseServer(raw); err == nil {
			t.Errorf("ParseServer(%q) accepted", raw)
		}
	}
}

func TestLoopback(t *testing.T) {
	for host, want := range map[string]bool{"localhost": true, "127.0.0.9": true, "::1": true, "[::1]": true, "10.0.0.1": false, "box.lan": false} {
		if Loopback(host) != want {
			t.Errorf("Loopback(%q) != %v", host, want)
		}
	}
}

func TestFromArgs(t *testing.T) {
	request, err := FromArgs([]string{"--flag", "servef-native://local/?project=%2Fp"}, "/wd")
	if err != nil || request.Link == nil || request.Link.Project != "/p" || request.Path != "" {
		t.Fatalf("link: %+v, %v", request, err)
	}
	request, err = FromArgs([]string{"-v", "code/../src"}, "/home/dee")
	if err != nil || request.Link != nil || request.Path != "/home/dee/src" {
		t.Fatalf("relative path: %+v, %v", request, err)
	}
	request, err = FromArgs([]string{"/abs/dir"}, "/wd")
	if err != nil || request.Path != "/abs/dir" {
		t.Fatalf("absolute path: %+v, %v", request, err)
	}
	if request, err = FromArgs(nil, "/wd"); err != nil || request.Link != nil || request.Path != "" {
		t.Fatalf("empty: %+v, %v", request, err)
	}
	if _, err = FromArgs([]string{"servef-native://nowhere/"}, "/wd"); err == nil {
		t.Fatal("invalid link accepted")
	}
}
