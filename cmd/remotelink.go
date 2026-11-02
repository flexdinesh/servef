package cmd

import (
	"net"
	"net/url"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"
)

// remoteHints identify this host in a servef://ssh link (decision 0069).
type remoteHints struct {
	Alias, Hostname, User, Addr string
	Port                        int // 0 unknown
}

// Host is the link's host segment: the alias when given, else the hostname.
func (h remoteHints) Host() string {
	if h.Alias != "" {
		return h.Alias
	}
	return h.Hostname
}

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,252}$`)

// validAlias reports whether alias is a usable --alias: a name ssh accepts as
// a Host token, with no leading dash.
func validAlias(alias string) bool { return aliasPattern.MatchString(alias) }

// hintsFromEnv reads the hints: the hostname, the user (user.Current, else
// $USER), and the server address and port of SSH_CONNECTION when it parses.
func hintsFromEnv(getenv func(string) string, hostname func() (string, error), username func() string, alias string) remoteHints {
	hints := remoteHints{Alias: alias}
	if name, err := hostname(); err == nil {
		hints.Hostname = name
	}
	hints.User = username()
	if hints.User == "" {
		hints.User = getenv("USER")
	}
	// SSH_CONNECTION is "client_ip client_port server_ip server_port".
	if fields := strings.Fields(getenv("SSH_CONNECTION")); len(fields) == 4 {
		ip := net.ParseIP(fields[2])
		port, err := strconv.Atoi(fields[3])
		if ip != nil && err == nil && port >= 1 && port <= 65535 {
			hints.Addr, hints.Port = ip.String(), port
		}
	}
	return hints
}

// currentUsername is the account name, or "" when it cannot be read.
func currentUsername() string {
	current, err := user.Current()
	if err != nil {
		return ""
	}
	return current.Username
}

// defaultRemoteHints reads this process's hints.
func defaultRemoteHints(getenv func(string) string, alias string) remoteHints {
	return hintsFromEnv(getenv, os.Hostname, currentUsername, alias)
}

// remoteLink returns servef://ssh/<host><route>?<query>&ssh-…: route is "/"
// or "/view", the web query keeps url.Values order, and the hint keys follow
// in a fixed order. Empty hints are omitted, as is port 22.
func remoteLink(h remoteHints, route string, query url.Values) string {
	parts := make([]string, 0, 5)
	if encoded := query.Encode(); encoded != "" {
		parts = append(parts, encoded)
	}
	hint := func(key, value string) {
		if value != "" {
			parts = append(parts, key+"="+url.QueryEscape(value))
		}
	}
	hint("ssh-hostname", h.Hostname)
	hint("ssh-user", h.User)
	hint("ssh-addr", h.Addr)
	if h.Port != 0 && h.Port != 22 {
		hint("ssh-port", strconv.Itoa(h.Port))
	}
	link := "servef://ssh/" + url.PathEscape(h.Host()) + route
	if len(parts) > 0 {
		link += "?" + strings.Join(parts, "&")
	}
	return link
}

// hyperlink wraps link in an OSC 8 hyperlink when tty; the visible text is
// the plain link either way.
func hyperlink(link string, tty bool) string {
	if !tty {
		return link
	}
	return "\x1b]8;;" + link + "\x1b\\" + link + "\x1b]8;;\x1b\\"
}
