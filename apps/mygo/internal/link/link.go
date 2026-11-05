// Package link parses what opens servef-mygo: servef-mygo:// links and
// command-line arguments. It is pure: no I/O.
//
// Links mirror servef:// (decisions 0063, 0076) with the scheme swapped:
//
//	servef-mygo://local/?project=<abs>&path=<rel>     the local daemon
//	servef-mygo://<host>:<port>/?project=…&path=…     an http server
//
// The route is "/" or "/view". The query and fragment are the web app's.
package link

import (
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// Scheme is the URL scheme servef-mygo registers.
const Scheme = "servef-mygo"

// Kind is the connection a target names.
type Kind int

const (
	// Local is the per-user daemon, shown in the window.
	Local Kind = iota
	// Server is a servef http server, opened in the browser.
	Server
)

// Target is a parsed link: where it points and what it shows there.
type Target struct {
	Kind Kind
	// Host and Port name a Server; empty and 0 for Local.
	Host string
	Port int
	// Route, RawQuery, and Fragment are the link's, for the web app. Fragment
	// stays percent-encoded: the web app decodes it itself.
	Route    string
	RawQuery string
	Fragment string
}

// ErrInvalid reports a link servef-mygo does not open.
var ErrInvalid = errors.New("not a servef-mygo link")

// Parse parses a servef-mygo:// link.
func Parse(raw string) (Target, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != Scheme || u.User != nil || u.Opaque != "" {
		return Target{}, ErrInvalid
	}
	route := u.Path
	if route == "" {
		route = "/"
	}
	if route != "/" && route != "/view" {
		return Target{}, ErrInvalid
	}
	target := Target{Route: route, RawQuery: u.RawQuery, Fragment: u.EscapedFragment()}
	if u.Host == "local" {
		return target, nil
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil || strings.EqualFold(host, "local") {
		return Target{}, ErrInvalid
	}
	target.Kind, target.Host, target.Port = Server, host, port
	return target, nil
}

// splitHostPort requires an explicit port from 1 to 65535.
func splitHostPort(hostport string) (string, int, error) {
	host, portText, err := net.SplitHostPort(hostport)
	if err != nil || host == "" {
		return "", 0, ErrInvalid
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != portText {
		return "", 0, ErrInvalid
	}
	return strings.ToLower(host), port, nil
}

// AppPath is the link's route, query, and fragment on the web app.
func (t Target) AppPath() string {
	u := url.URL{Path: t.Route, RawQuery: t.RawQuery}
	if t.Fragment == "" {
		return u.String()
	}
	return u.String() + "#" + t.Fragment
}

// Origin is a Server target's http origin, as the server prints it.
func (t Target) Origin() string {
	return "http://" + net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
}

// Request is what one launch, or a second instance, asks for.
type Request struct {
	// Link is set when the arguments carry a servef-mygo:// link.
	Link *Target
	// Path is an absolute directory to open through the daemon, as
	// `servef [path]` does; empty when none was given.
	Path string
}

// FromArgs reads a command line, without the program name: the first
// servef-mygo:// argument wins; otherwise the first argument that is not a
// flag names a path, made absolute against workingDir. A link that does not
// parse is an error.
func FromArgs(args []string, workingDir string) (Request, error) {
	for _, arg := range args {
		if strings.HasPrefix(arg, Scheme+"://") {
			target, err := Parse(arg)
			if err != nil {
				return Request{}, err
			}
			return Request{Link: &target}, nil
		}
	}
	for _, arg := range args {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		path := arg
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		return Request{Path: filepath.Clean(path)}, nil
	}
	return Request{}, nil
}
