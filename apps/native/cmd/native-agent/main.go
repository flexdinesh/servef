//go:build agent

// Command native-agent serves the native app headless for agents
// (decision 0086): the real view over the workspace model in a
// packages/mygo agent session, against the shared API fixtures by
// default. mygo-agent and scenarios drive it over the socket at
// --socket or $MYGO_AGENT_SOCKET.
//
//	go run -tags agent ./cmd/native-agent --socket .dev/native-agent/agent.sock
//
// With --fixtures-server it serves only the fixtures, as a servef server
// over TCP, for a built app to connect to (the e2e smoke).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/flexdinesh/servef/apps/native/internal/agenthost"
	"github.com/flexdinesh/servef/apps/native/internal/fakeservef"
)

func main() {
	var opts agenthost.Options
	socket := flag.String("socket", os.Getenv("MYGO_AGENT_SOCKET"), "agent socket to listen on")
	flag.StringVar(&opts.Fixtures, "fixtures", "", "API fixture directory (default testdata/api/default)")
	flag.StringVar(&opts.Daemon, "daemon", "", "a running servef daemon's socket instead of fixtures")
	flag.StringVar(&opts.Server, "server", "", "a servef server's address instead of fixtures")
	flag.StringVar(&opts.Link, "link", "", "servef-native:// link to open at start")
	flag.BoolVar(&opts.ConnectPage, "connect-page", false, "start on the connect page")
	flag.IntVar(&opts.Width, "width", 1100, "window width in DIPs")
	flag.IntVar(&opts.Height, "height", 700, "window height in DIPs")
	flag.BoolVar(&opts.Dark, "dark", false, "dark appearance")
	fixturesServer := flag.String("fixtures-server", "", "serve only the fixtures over TCP at host:port")
	flag.Parse()
	if *fixturesServer != "" {
		serveFixtures(*fixturesServer, opts.Fixtures)
		return
	}
	if *socket == "" {
		log.Fatal("native-agent: --socket or MYGO_AGENT_SOCKET is required")
	}
	if err := os.MkdirAll(filepath.Dir(*socket), 0o700); err != nil {
		log.Fatal(err)
	}
	host, err := agenthost.Start(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer host.Close()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		host.Session.Close()
	}()
	fmt.Fprintln(os.Stderr, "native-agent: serving", *socket)
	if err := host.Session.ListenAndServe(*socket); err != nil {
		log.Fatal(err)
	}
}

// serveFixtures serves the fixtures as a servef server until interrupted.
func serveFixtures(addr, fixtures string) {
	server, err := fakeservef.StartServer(addr)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()
	if fixtures == "" {
		fixtures = fakeservef.FixtureDir()
	}
	if err := server.LoadFixtures(fixtures); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintln(os.Stderr, "native-agent: fixtures at", server.URL)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
}
