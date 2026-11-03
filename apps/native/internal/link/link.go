// Package link parses what opens servef-native: servef-native:// links and
// command-line arguments (decision 0082). It is pure: no I/O.
//
// Links mirror servef:// (decisions 0063, 0076) with the scheme swapped:
//
//	servef-native://local/?project=<abs>&path=<rel>     the local daemon
//	servef-native://<host>:<port>/?project=…&path=…     an http server
//
// The route is "/" or "/view"; review=working opens the uncommitted changes.
package link

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// Scheme is the URL scheme servef-native registers.
const Scheme = "servef-native"

// Kind is the connection a target names.
type Kind int

const (
	// Local is the per-user daemon over its Unix socket.
	Local Kind = iota
	// Server is a servef http server over TCP.
	Server
)

// Target is a parsed link: where to connect and what to show there.
type Target struct {
	Kind Kind
	// Host and Port name a Server; empty and 0 for Local.
	Host string
	Port int
	// Project is the project ID (an absolute path); Path a project-relative
	// document path. Either may be empty.
	Project string
	Path    string
	// Review opens the project's uncommitted changes.
	Review bool
}

// ErrInvalid reports a link servef-native does not open.
var ErrInvalid = errors.New("not a servef-native link")

// Parse parses a servef-native:// link.
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
	query := u.Query()
	target := Target{Project: query.Get("project"), Path: query.Get("path"), Review: query.Get("review") == "working"}
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

// Origin is a Server target's http origin, as the server prints it.
func (t Target) Origin() string {
	return "http://" + net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
}

// ParseServer reads an address typed or pasted to connect to: an http URL
// ("http://host:port/…", where any route and query are ignored), a
// servef:// or servef-native:// server link, or a bare "host:port".
func ParseServer(raw string) (Target, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return Target{}, errors.New("enter a server address, such as http://localhost:7971")
	}
	if !strings.Contains(text, "://") {
		text = "http://" + text
	}
	u, err := url.Parse(text)
	if err != nil || u.User != nil {
		return Target{}, fmt.Errorf("%q is not a server address", raw)
	}
	switch u.Scheme {
	case "http", "servef", Scheme:
	default:
		return Target{}, fmt.Errorf("%q is not a servef server address", raw)
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil || strings.EqualFold(host, "local") {
		return Target{}, fmt.Errorf("%q needs a host and port, such as localhost:7971", raw)
	}
	query := u.Query()
	return Target{Kind: Server, Host: host, Port: port, Project: query.Get("project"), Path: query.Get("path")}, nil
}

// Loopback reports a host on this machine: localhost or a loopback IP.
func Loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// Request is what one launch, or a second instance, asks for.
type Request struct {
	// Link is set when the arguments carry a servef-native:// link.
	Link *Target
	// Path is an absolute directory to open through the daemon, as
	// `servef [path]` does; empty when none was given.
	Path string
}

// FromArgs reads a command line, without the program name: the first
// servef-native:// argument wins; otherwise the first argument that is not
// a flag names a path, made absolute against workingDir. A link that does
// not parse is an error.
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
