package cmd

import (
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

// noUI reports whether this machine cannot show a UI to the user at its
// terminal (decision 0075): an SSH session, or Linux without a display.
func noUI(goos string, getenv func(string) string) bool {
	if inSSHSession(getenv) {
		return true
	}
	return goos == "linux" && getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == ""
}

func inSSHSession(getenv func(string) string) bool {
	for _, name := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		if getenv(name) != "" {
			return true
		}
	}
	return false
}

// routeAddresses lie outside every local network (documentation ranges); a
// UDP socket "connected" to one makes the kernel pick the default route's
// interface. Nothing is sent.
var routeAddresses = []string{"192.0.2.1:9", "[2001:db8::1]:9"}

// detectHost returns the IP a no-UI server binds and prints: the server
// address of SSH_CONNECTION ("client_ip client_port server_ip server_port"),
// else route's outbound address, else "" (the caller falls back to
// localhost).
func detectHost(getenv func(string) string, route func() net.IP) string {
	if fields := strings.Fields(getenv("SSH_CONNECTION")); len(fields) == 4 {
		if ip := net.ParseIP(fields[2]); usableAddress(ip) {
			return ip.String()
		}
	}
	if ip := route(); usableAddress(ip) {
		return ip.String()
	}
	return ""
}

// usableAddress reports whether ip can be bound and dialed from another
// machine without a zone.
func usableAddress(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() && !ip.IsMulticast()
}

// outboundAddress is the local address of the default route, or nil.
func outboundAddress() net.IP {
	for _, address := range routeAddresses {
		conn, err := net.Dial("udp", address)
		if err != nil {
			continue
		}
		local, ok := conn.LocalAddr().(*net.UDPAddr)
		_ = conn.Close()
		if ok && usableAddress(local.IP) {
			return local.IP
		}
	}
	return nil
}

// loopbackHost reports whether host (an IP or "localhost") only accepts
// connections from this machine.
func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// desktopURL is a server URL's desktop form: the same URL with the servef
// scheme (decision 0075).
func desktopURL(webURL string) string {
	if rest, ok := strings.CutPrefix(webURL, "http://"); ok {
		return "servef://" + rest
	}
	return webURL
}

// serverURL is the web URL of a server bound to host and port.
func serverURL(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/"
}

// hyperlink wraps link in an OSC 8 hyperlink when tty; the visible text is
// the plain link either way.
func hyperlink(link string, tty bool) string {
	if !tty {
		return link
	}
	return "\x1b]8;;" + link + "\x1b\\" + link + "\x1b]8;;\x1b\\"
}

// linkTerminal reports whether links written to w may be OSC 8 hyperlinks:
// w is a terminal and TERM is not dumb.
func linkTerminal(w io.Writer, getenv func(string) string) bool {
	return isTerminal(w) && getenv("TERM") != "dumb"
}

// isTerminal reports whether w is a terminal device.
func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
