package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/flexdinesh/servef/internal/desktopapp"
	"github.com/flexdinesh/servef/pkg/connection"
	"github.com/flexdinesh/servef/pkg/contract"
	"github.com/spf13/cobra"
)

type connectOptions struct {
	text   string
	host   string
	noOpen bool
}

// connectDeps are the effects `servef connect` uses; tests replace them.
type connectDeps struct {
	goos      string
	getenv    func(string) string
	terminal  func(io.Writer) bool
	probe     func(context.Context, connection.TLS) (connection.TLS, contract.RemoteInfo, error)
	storePath func() (string, error)
	installed func(context.Context) (bool, error)
	openApp   func(context.Context, string) error
	now       func() time.Time
}

func defaultConnectDeps() connectDeps {
	return connectDeps{
		goos:      runtime.GOOS,
		getenv:    os.Getenv,
		terminal:  isTerminal,
		probe:     connection.Probe,
		storePath: connection.DefaultStorePath,
		installed: desktopapp.Installed,
		openApp:   desktopapp.Open,
		now:       time.Now,
	}
}

func newConnectCommand() *cobra.Command {
	var opts connectOptions
	command := &cobra.Command{
		Use:           "connect <string>",
		Short:         "Open a server started with servef --remote in the desktop app",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, args []string) error {
			opts.text = args[0]
			return runConnect(command.Context(), opts, command.OutOrStdout(), command.ErrOrStderr(), defaultConnectDeps())
		},
	}
	command.Flags().StringVar(&opts.host, "host", "", "connect through this address instead, such as localhost for an ssh tunnel")
	command.Flags().BoolVar(&opts.noOpen, "no-open", false, "save the connection and print its link instead of opening it")
	return command
}

// runConnect checks the server answers to the string, saves the connection,
// and opens it. The link names the connection; the secret stays in the store.
func runConnect(ctx context.Context, opts connectOptions, stdout, stderr io.Writer, deps connectDeps) error {
	endpoint, err := connection.Parse(opts.text)
	if err != nil {
		return errors.New("not a servef connection string: expected the host:port/credential that `servef <path> --remote` prints")
	}
	if opts.host != "" {
		host, err := connection.ParseHost(opts.host)
		if err != nil {
			return fmt.Errorf("--host: %w", err)
		}
		endpoint.Hosts = []string{host}
	}
	working, info, err := deps.probe(ctx, endpoint)
	if err != nil {
		return connectFailure(err, endpoint)
	}
	path, err := deps.storePath()
	if err != nil {
		return err
	}
	saved := connection.Saved{
		ID:         working.Credential.ID(),
		Label:      info.Name,
		Hosts:      working.Hosts,
		Port:       working.Port,
		Credential: working.Credential.Encode(),
		AddedAt:    deps.now().UTC().Truncate(time.Second),
	}
	if err := connection.NewStore(path).Put(saved); err != nil {
		return err
	}

	target := "servef://" + saved.ID + "/"
	link := hyperlink(target, deps.terminal(stdout) && deps.getenv("TERM") != "dumb")
	connected := func() {
		_, _ = fmt.Fprintf(stdout, "connected to %s (servef %s)\ndesktop: %s\n", info.Name, info.Version, link)
	}
	if opts.noOpen {
		connected()
		return nil
	}
	unavailable := func(notice string) error {
		_, _ = fmt.Fprintf(stderr, "%s\n%s", notice, desktopapp.InstallHint(deps.goos))
		web, err := working.WebURL()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "connected to %s (servef %s)\nweb: %s\n", info.Name, info.Version, web)
		return nil
	}
	if deps.goos == "linux" {
		installed, err := deps.installed(ctx)
		if err != nil {
			return unavailable(fmt.Sprintf("could not detect the servef desktop app (%v)", err))
		}
		if !installed {
			return unavailable(desktopapp.ErrNotInstalled.Error())
		}
	}
	if err := deps.openApp(ctx, target); err != nil {
		if errors.Is(err, desktopapp.ErrNotInstalled) {
			return unavailable(desktopapp.ErrNotInstalled.Error())
		}
		return unavailable(fmt.Sprintf("could not open the servef desktop app (%v)", err))
	}
	connected()
	return nil
}

// connectFailure says which of the three things went wrong and what to do.
func connectFailure(err error, endpoint connection.TLS) error {
	hosts := strings.Join(endpoint.Hosts, ", ")
	switch {
	case errors.Is(err, connection.ErrFingerprint):
		return fmt.Errorf("the server at %s has a different identity: its credential was rotated, or something else answers there. Run `servef <path> --remote` on it and use the new string", hosts)
	case errors.Is(err, connection.ErrToken):
		return fmt.Errorf("the server at %s refused this string's token. Run `servef <path> --remote` on it and use the new string", hosts)
	case errors.Is(err, connection.ErrUnreachable):
		return fmt.Errorf(`could not reach %s on port %d.
Check `+"`servef remote status`"+` on the server: it stops when no UI needs it, so `+"`servef <path> --remote`"+` may need to run again.
A firewall may block the port. To tunnel instead:
  ssh -L %d:localhost:%d <server>
  servef connect <string> --host localhost`, hosts, endpoint.Port, endpoint.Port, endpoint.Port)
	}
	return err
}
