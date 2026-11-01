package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// remoteLinkCase is one testdata/remote-links.json entry, shared with the
// desktop app (decision 0069): hints generate url; parsed is what the app
// reads back from it.
type remoteLinkCase struct {
	Name  string `json:"name"`
	Hints struct {
		Alias    string `json:"alias"`
		Hostname string `json:"hostname"`
		User     string `json:"user"`
		Addr     string `json:"addr"`
		Port     int    `json:"port"`
	} `json:"hints"`
	Route  string            `json:"route"`
	Query  map[string]string `json:"query"`
	URL    string            `json:"url"`
	Parsed parsedRemoteLink  `json:"parsed"`
}

type parsedRemoteLink struct {
	Host     string `json:"host"`
	Hostname string `json:"hostname,omitempty"`
	User     string `json:"user,omitempty"`
	Addr     string `json:"addr,omitempty"`
	Port     int    `json:"port,omitempty"`
}

func readRemoteLinkCases(t *testing.T) []remoteLinkCase {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "testdata", "remote-links.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var cases []remoteLinkCase
	if err := decoder.Decode(&cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no remote link cases")
	}
	return cases
}

// parseRemoteLink reads a link as the desktop app does: the host segment,
// the route, the web query, and the ssh- hints.
func parseRemoteLink(raw string) (parsedRemoteLink, string, map[string]string, error) {
	link, err := url.Parse(raw)
	if err != nil {
		return parsedRemoteLink{}, "", nil, err
	}
	if link.Scheme != "servef" || link.Host != "ssh" {
		return parsedRemoteLink{}, "", nil, errors.New("not a servef://ssh link")
	}
	segment, route, ok := strings.Cut(strings.TrimPrefix(link.EscapedPath(), "/"), "/")
	if !ok {
		return parsedRemoteLink{}, "", nil, errors.New("no route")
	}
	host, err := url.PathUnescape(segment)
	if err != nil {
		return parsedRemoteLink{}, "", nil, err
	}
	values, err := url.ParseQuery(link.RawQuery)
	if err != nil {
		return parsedRemoteLink{}, "", nil, err
	}
	parsed := parsedRemoteLink{Host: host, Hostname: values.Get("ssh-hostname"), User: values.Get("ssh-user"), Addr: values.Get("ssh-addr")}
	if port := values.Get("ssh-port"); port != "" {
		if parsed.Port, err = strconv.Atoi(port); err != nil {
			return parsedRemoteLink{}, "", nil, err
		}
	}
	query := map[string]string{}
	for key := range values {
		if !strings.HasPrefix(key, "ssh-") {
			query[key] = values.Get(key)
		}
	}
	return parsed, "/" + route, query, nil
}

func TestRemoteLinkMatchesTheSharedFixture(t *testing.T) {
	for _, test := range readRemoteLinkCases(t) {
		t.Run(test.Name, func(t *testing.T) {
			hints := remoteHints{Alias: test.Hints.Alias, Hostname: test.Hints.Hostname, User: test.Hints.User, Addr: test.Hints.Addr, Port: test.Hints.Port}
			query := url.Values{}
			for key, value := range test.Query {
				query.Set(key, value)
			}
			if got := remoteLink(hints, test.Route, query); got != test.URL {
				t.Errorf("remoteLink = %s\nwant         %s", got, test.URL)
			}
			parsed, route, gotQuery, err := parseRemoteLink(test.URL)
			if err != nil {
				t.Fatal(err)
			}
			if parsed != test.Parsed || route != test.Route || !reflect.DeepEqual(gotQuery, test.Query) {
				t.Errorf("parsed %+v %s %v, want %+v %s %v", parsed, route, gotQuery, test.Parsed, test.Route, test.Query)
			}
		})
	}
}

func TestHintsFromEnv(t *testing.T) {
	hostname := func() (string, error) { return "devbox", nil }
	username := func() string { return "dee" }
	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	for _, test := range []struct {
		name       string
		connection string
		addr       string
		port       int
	}{
		{"IPv4", "10.0.0.9 51234 10.0.0.5 22", "10.0.0.5", 22},
		{"IPv6 is canonical", "2001:db8::9 51234 2001:DB8:0::5 2222", "2001:db8::5", 2222},
		{"missing", "", "", 0},
		{"too few fields", "10.0.0.9 51234 10.0.0.5", "", 0},
		{"bad address", "10.0.0.9 51234 devbox 22", "", 0},
		{"zoned address", "fe80::9 51234 fe80::5%eth0 22", "", 0},
		{"port zero", "10.0.0.9 51234 10.0.0.5 0", "", 0},
		{"port too large", "10.0.0.9 51234 10.0.0.5 65536", "", 0},
		{"port not a number", "10.0.0.9 51234 10.0.0.5 ssh", "", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := hintsFromEnv(env(map[string]string{"SSH_CONNECTION": test.connection}), hostname, username, "")
			want := remoteHints{Hostname: "devbox", User: "dee", Addr: test.addr, Port: test.port}
			if got != want {
				t.Errorf("hints = %+v, want %+v", got, want)
			}
		})
	}

	// The alias names the link; $USER stands in for an unreadable account;
	// a failed hostname is empty.
	got := hintsFromEnv(env(map[string]string{"USER": "env-user"}), func() (string, error) { return "", errors.New("no name") }, func() string { return "" }, "work")
	if want := (remoteHints{Alias: "work", User: "env-user"}); got != want {
		t.Errorf("hints = %+v, want %+v", got, want)
	}
	if got.Host() != "work" {
		t.Errorf("host = %q, want the alias", got.Host())
	}
}

func TestHyperlinkFramesOnlyTerminals(t *testing.T) {
	link := "servef://ssh/devbox/?project=%2Fsrv"
	if got := hyperlink(link, false); got != link {
		t.Errorf("pipe output = %q, want the plain link", got)
	}
	if got, want := hyperlink(link, true), "\x1b]8;;"+link+"\x1b\\"+link+"\x1b]8;;\x1b\\"; got != want {
		t.Errorf("terminal output = %q, want %q", got, want)
	}
}

func TestValidAlias(t *testing.T) {
	for alias, want := range map[string]bool{
		"devbox":                       true,
		"Work_Box.2":                   true,
		"9host":                        true,
		"a" + strings.Repeat("b", 252): true,
		"a" + strings.Repeat("b", 253): false,
		"":                             false,
		"-oProxyCommand=x":             false,
		".hidden":                      false,
		"user@host":                    false,
		"dev box":                      false,
		"host/route":                   false,
		"host%25":                      false,
		"hôte":                         false,
	} {
		if got := validAlias(alias); got != want {
			t.Errorf("validAlias(%q) = %t, want %t", alias, got, want)
		}
	}
}
