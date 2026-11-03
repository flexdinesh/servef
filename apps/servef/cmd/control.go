package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/flexdinesh/servef/internal/control"
	"github.com/spf13/cobra"
)

// controlListTimeout bounds the concurrent Running checks of control list.
const controlListTimeout = 2 * time.Second

// listedServer is one running CLI server in `servef control list --json`
// (decision 0071). Tokens never leave internal/control.
type listedServer struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Origin string `json:"origin"`
}

// newControlCommand builds the hidden `servef control` plumbing: `list`
// prints the running registered CLI servers for the desktop app.
// controlDir "" uses control.DefaultDirectory().
func newControlCommand(controlDir string) *cobra.Command {
	command := &cobra.Command{
		Use:           "control",
		Short:         "Inspect running servef servers (plumbing for the desktop app)",
		Args:          cobra.NoArgs,
		Hidden:        true,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	var asJSON bool
	list := &cobra.Command{
		Use:           "list",
		Short:         "List running servef servers",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, _ []string) error {
			directory := controlDir
			if directory == "" {
				var err error
				if directory, err = control.DefaultDirectory(); err != nil {
					return err
				}
			}
			return listServers(command.Context(), control.NewRegistry(directory), asJSON, command.OutOrStdout())
		},
	}
	list.Flags().BoolVar(&asJSON, "json", false, "print the servers as JSON")
	command.AddCommand(list)
	return command
}

// listServers prints the registered servers whose authenticated control check
// passes, in registry order: JSON {"servers":[...]} or one origin per line.
func listServers(ctx context.Context, registry control.Registry, asJSON bool, stdout io.Writer) error {
	remotes, err := registry.List()
	if err != nil {
		return fmt.Errorf("list servers: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, controlListTimeout)
	defer cancel()
	running := make([]bool, len(remotes))
	var wait sync.WaitGroup
	for index, remote := range remotes {
		wait.Add(1)
		go func() {
			defer wait.Done()
			running[index] = remote.Running(ctx)
		}()
	}
	wait.Wait()
	servers := make([]listedServer, 0, len(remotes))
	for index, remote := range remotes {
		if running[index] {
			servers = append(servers, listedServer{Host: remote.Host(), Port: remote.Port(), Origin: remote.Origin()})
		}
	}
	if asJSON {
		return json.NewEncoder(stdout).Encode(struct {
			Servers []listedServer `json:"servers"`
		}{servers})
	}
	for _, server := range servers {
		if _, err := fmt.Fprintln(stdout, server.Origin); err != nil {
			return err
		}
	}
	return nil
}
