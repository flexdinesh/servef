package link

import (
	"errors"
	"net/url"
	"testing"
)

func TestParse(t *testing.T) {
	for raw, want := range map[string]Target{
		"servef://local/":                          {Route: "/"},
		"servef://local":                           {Route: "/"},
		"servef://local/view?project=%2Fa&path=b":  {Route: "/view", RawQuery: "project=%2Fa&path=b"},
		"servef://local/?review=working#L12-L20":   {Route: "/", RawQuery: "review=working", Fragment: "L12-L20"},
		"servef://LOCALHOST:7971/view?project=%2F": {Kind: Server, Host: "localhost", Port: 7971, Route: "/view", RawQuery: "project=%2F"},
		"servef://[::1]:80/":                       {Kind: Server, Host: "::1", Port: 80, Route: "/"},
	} {
		got, err := Parse(raw)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"servef-mygo://local/", "http://local/", "servef:local", "servef://user@local/",
		"servef://local/other", "servef://local/_daemon/status", "servef://host/", "servef://host:0/",
		"servef://host:70000/", "servef://host:07/", "servef://local:80/", "%zz",
	} {
		if _, err := Parse(raw); !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) = %v, want ErrInvalid", raw, err)
		}
	}
}

func TestAppPathKeepsQueryAndFragment(t *testing.T) {
	target, err := Parse("servef://local/view?project=%2Fa%20b&path=c%2Fd.md#L3")
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
		"servef://local/view?project=%2Fa#f=a%26b.md":    "/view?project=%2Fa#f=a%26b.md",
		"servef://local/view#f=a%26b.md&l=3":             "/view#f=a%26b.md&l=3",
		"servef://local/view#f=dir%2Fname%20with%20x.md": "/view#f=dir%2Fname%20with%20x.md",
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
		"servef://localhost:7971/": "http://localhost:7971",
		"servef://[::1]:80/":       "http://[::1]:80",
	} {
		target, err := Parse(raw)
		if err != nil || target.Origin() != want {
			t.Errorf("Origin of %q = %q, %v", raw, target.Origin(), err)
		}
	}
}

// The CLI builds these (apps/servef/cmd: app.go, walkthrough.go, and
// address.go; app_test.go and address_test.go pin the same URLs): the app
// must open each one, query intact.
func TestCLIURLs(t *testing.T) {
	for raw, want := range map[string]struct {
		kind    Kind
		path    string
		project string
		origin  string
	}{
		"servef://local/?project=%2Fwork%2Fmy+repo":           {Local, "/?project=%2Fwork%2Fmy+repo", "/work/my repo", ""},
		"servef://local/view?project=%2Fwork&wt=tour":         {Local, "/view?project=%2Fwork&wt=tour", "/work", ""},
		"servef://localhost:4173/":                            {Server, "/", "", "http://localhost:4173"},
		"servef://10.0.0.5:7971/":                             {Server, "/", "", "http://10.0.0.5:7971"},
		"servef://[2001:db8::5]:7971/view?wt=tour":            {Server, "/view?wt=tour", "", "http://[2001:db8::5]:7971"},
		"servef://10.0.0.5:7971/view?project=%2Fwork&wt=tour": {Server, "/view?project=%2Fwork&wt=tour", "/work", "http://10.0.0.5:7971"},
	} {
		target, err := Parse(raw)
		if err != nil {
			t.Errorf("Parse(%q) = %v", raw, err)
			continue
		}
		query, err := url.ParseQuery(target.RawQuery)
		if err != nil {
			t.Errorf("query of %q: %v", raw, err)
		}
		if target.Kind != want.kind || target.AppPath() != want.path || query.Get("project") != want.project {
			t.Errorf("Parse(%q) = %+v, AppPath %q, project %q; want %+v", raw, target, target.AppPath(), query.Get("project"), want)
		}
		if want.kind == Server && target.Origin() != want.origin {
			t.Errorf("Origin of %q = %q, want %q", raw, target.Origin(), want.origin)
		}
	}
}

func TestFromArgs(t *testing.T) {
	request, err := FromArgs([]string{"dir", "servef://local/"}, "/work")
	if err != nil || request.Link == nil || request.Path != "" {
		t.Errorf("a link wins over a path: %+v, %v", request, err)
	}
	if _, err := FromArgs([]string{"servef://nope/"}, "/work"); err == nil {
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
