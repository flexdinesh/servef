package cmd

import (
	"net"
	"testing"
)

func TestDetectHostPrefersTheSSHServerAddress(t *testing.T) {
	route := func(ip string) func() net.IP { return func() net.IP { return net.ParseIP(ip) } }
	env := func(connection string) func(string) string {
		return func(name string) string {
			if name == "SSH_CONNECTION" {
				return connection
			}
			return ""
		}
	}
	for _, test := range []struct {
		name       string
		connection string
		route      string
		want       string
	}{
		{"IPv4 server address", "10.0.0.9 51234 10.0.0.5 22", "192.168.1.20", "10.0.0.5"},
		{"IPv6 server address", "2001:db8::9 51234 2001:db8::5 22", "", "2001:db8::5"},
		{"no SSH uses the route", "", "192.168.1.20", "192.168.1.20"},
		{"malformed SSH uses the route", "10.0.0.9 51234", "192.168.1.20", "192.168.1.20"},
		{"loopback SSH uses the route", "127.0.0.1 51234 127.0.0.1 22", "192.168.1.20", "192.168.1.20"},
		{"link-local SSH uses the route", "fe80::9 51234 fe80::5 22", "192.168.1.20", "192.168.1.20"},
		{"nothing usable", "", "127.0.0.1", ""},
		{"no route", "", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := detectHost(env(test.connection), route(test.route)); got != test.want {
				t.Errorf("detectHost() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLoopbackHost(t *testing.T) {
	for host, want := range map[string]bool{"localhost": true, "LOCALHOST": true, "127.0.0.1": true, "127.1.2.3": true, "::1": true, "10.0.0.5": false, "0.0.0.0": false, "::": false} {
		if got := loopbackHost(host); got != want {
			t.Errorf("loopbackHost(%q) = %t, want %t", host, got, want)
		}
	}
}

func TestDesktopURLSwapsTheScheme(t *testing.T) {
	for in, want := range map[string]string{
		"http://localhost:7971/":                 "servef://localhost:7971/",
		"http://[2001:db8::5]:7971/view?wt=tour": "servef://[2001:db8::5]:7971/view?wt=tour",
		"servef://local/?project=%2Fwork":        "servef://local/?project=%2Fwork",
	} {
		if got := desktopURL(in); got != want {
			t.Errorf("desktopURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHyperlinkFramesOnlyTerminals(t *testing.T) {
	const link = "servef://10.0.0.5:7971/"
	if got := hyperlink(link, false); got != link {
		t.Errorf("hyperlink(tty=false) = %q", got)
	}
	if got, want := hyperlink(link, true), "\x1b]8;;"+link+"\x1b\\"+link+"\x1b]8;;\x1b\\"; got != want {
		t.Errorf("hyperlink(tty=true) = %q, want %q", got, want)
	}
}
