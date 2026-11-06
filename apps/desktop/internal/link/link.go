// Package link parses what opens srcz-desktop: srcz:// links and
// command-line arguments. It is pure: no I/O.
//
// The CLI prints these links (decisions 0063, 0098):
//
//	srcz://local/?project=<abs>&path=<rel>   the local daemon
//	srcz://<id>/?project=<abs>&path=<rel>    a saved connection
//
// A link names a connection and carries no secret: `srcz connect` saves the
// credential. The route is "/" or "/view". The query and fragment are the web
// app's.
package link

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/flexdinesh/srcz/pkg/connection"
)

// Scheme is the URL scheme srcz-desktop registers.
const Scheme = "srcz"

// Target is a parsed link: where it points and what it shows there.
type Target struct {
	// Connection is connection.Local or a saved connection's id.
	Connection string
	// Route, RawQuery, and Fragment are the link's, for the web app. Fragment
	// stays percent-encoded: the web app decodes it itself.
	Route    string
	RawQuery string
	Fragment string
}

// ErrInvalid reports a link srcz-desktop does not open.
var ErrInvalid = errors.New("not a srcz link")

// Parse parses a srcz:// link.
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
	if u.Host != connection.Local && !connection.IsID(u.Host) {
		return Target{}, ErrInvalid
	}
	return Target{Connection: u.Host, Route: route, RawQuery: u.RawQuery, Fragment: u.EscapedFragment()}, nil
}

// AppPath is the link's route, query, and fragment on the web app.
func (t Target) AppPath() string {
	u := url.URL{Path: t.Route, RawQuery: t.RawQuery}
	if t.Fragment == "" {
		return u.String()
	}
	return u.String() + "#" + t.Fragment
}

// Request is what one launch, or a second instance, asks for.
type Request struct {
	// Link is set when the arguments carry a srcz:// link.
	Link *Target
	// Path is an absolute directory to open through the daemon, as
	// `srcz [path]` does; empty when none was given.
	Path string
}

// FromArgs reads a command line, without the program name: the first
// srcz:// argument wins; otherwise the first argument that is not a
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
