package link

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	for raw, want := range map[string]Target{
		"servef-mygo://local/":                          {Route: "/"},
		"servef-mygo://local":                           {Route: "/"},
		"servef-mygo://local/view?project=%2Fa&path=b":  {Route: "/view", RawQuery: "project=%2Fa&path=b"},
		"servef-mygo://local/?review=working#L12-L20":   {Route: "/", RawQuery: "review=working", Fragment: "L12-L20"},
		"servef-mygo://LOCALHOST:7971/view?project=%2F": {Kind: Server, Host: "localhost", Port: 7971, Route: "/view", RawQuery: "project=%2F"},
		"servef-mygo://[::1]:80/":                       {Kind: Server, Host: "::1", Port: 80, Route: "/"},
	} {
		got, err := Parse(raw)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"servef://local/", "http://local/", "servef-mygo:local", "servef-mygo://user@local/",
		"servef-mygo://local/other", "servef-mygo://local/_daemon/status", "servef-mygo://host/", "servef-mygo://host:0/",
		"servef-mygo://host:70000/", "servef-mygo://host:07/", "servef-mygo://local:80/", "%zz",
	} {
		if _, err := Parse(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) = %v, want ErrInvalid", raw, err)
		}
	}
}

func TestAppPathKeepsQueryAndFragment(t *testing.T) {
	target, err := Parse("servef-mygo://local/view?project=%2Fa%20b&path=c%2Fd.md#L3")
	if err != nil {
		t.Fatal(err)
	}
	if got := target.AppPath(); got != "/view?project=%2Fa%20b&path=c%2Fd.md#L3" {
		t.Errorf("AppPath = %q", got)
	}
}

// A review fragment's path is percent-encoded: "&" in a name must stay encoded,
// or parseReviewFragment reads it as the end of the path.
func TestAppPathKeepsAnEncodedFragment(t *testing.T) {
	for raw, want := range map[string]string{
		"servef-mygo://local/view?project=%2Fa#f=a%26b.md":    "/view?project=%2Fa#f=a%26b.md",
		"servef-mygo://local/view#f=a%26b.md&l=3":             "/view#f=a%26b.md&l=3",
		"servef-mygo://local/view#f=dir%2Fname%20with%20x.md": "/view#f=dir%2Fname%20with%20x.md",
	} {
		target, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := target.AppPath(); got != want {
			t.Errorf("AppPath of %q = %q, want %q", raw, got, want)
		}
	}
}

func TestServerOrigin(t *testing.T) {
	for raw, want := range map[string]string{
		"servef-mygo://localhost:7971/": "http://localhost:7971",
		"servef-mygo://[::1]:80/":       "http://[::1]:80",
	} {
		target, err := Parse(raw)
		if err != nil || target.Origin() != want {
			t.Errorf("Origin of %q = %q, %v", raw, target.Origin(), err)
		}
	}
}

func TestFromArgs(t *testing.T) {
	request, err := FromArgs([]string{"dir", "servef-mygo://local/"}, "/work")
	if err != nil || request.Link == nil || request.Path != "" {
		t.Errorf("a link wins over a path: %+v, %v", request, err)
	}
	if _, err := FromArgs([]string{"servef-mygo://nope/"}, "/work"); err == nil {
		t.Error("an invalid link was accepted")
	}
	for name, test := range map[string]struct {
		args []string
		want string
	}{
		"relative":  {[]string{"a/../b"}, "/work/b"},
		"absolute":  {[]string{"/x/"}, "/x"},
		"flags":     {[]string{"--flag", "", "c"}, "/work/c"},
		"no paths":  {[]string{"--flag"}, ""},
		"no args":   {nil, ""},
		"the first": {[]string{"d", "e"}, "/work/d"},
	} {
		request, err := FromArgs(test.args, "/work")
		if err != nil || request.Path != test.want {
			t.Errorf("%s: %+v, %v; want %q", name, request, err, test.want)
		}
	}
}
