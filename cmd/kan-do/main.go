// Command kan-do is a kanban board for you and your coding agents: the board in your
// browser, and an MCP server agents use to work on the same tasks.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/azmainadel/kan-do/internal/home"
	"github.com/azmainadel/kan-do/internal/mcp"
	"github.com/azmainadel/kan-do/internal/version"
	"github.com/azmainadel/kan-do/internal/web"
)

func main() {
	// Ctrl-C, or SIGTERM from launchd or systemd stopping the service.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := root().ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kan-do: %v\n", err)
		os.Exit(1)
	}
}

func root() *cobra.Command {
	var (
		homeFlag string
		port     uint16
		noOpen   bool
	)
	open := func() (*home.Home, error) {
		dir, err := home.Resolve(homeFlag)
		if err != nil {
			return nil, err
		}
		return home.Open(dir)
	}

	cmd := &cobra.Command{
		Use:     "kan-do",
		Short:   "A kanban board for you and your coding agents",
		Long:    "kan-do is a kanban board for you and your coding agents.\n\nRun `kan-do` to open the board. Agents connect with `kan-do mcp`.",
		Version: version.Version,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := open()
			if err != nil {
				return err
			}
			return web.Serve(cmd.Context(), h, port, !noOpen)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&homeFlag, "home", "", "data folder (default $"+home.EnvHome+", then ~/kan-do)")
	cmd.Flags().Uint16Var(&port, "port", 0, "port for the board (default from config.toml)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "start the board without opening the browser")

	cmd.AddCommand(&cobra.Command{
		Use:   "mcp",
		Short: "Run the MCP server on stdio (agents launch this, not you)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := open()
			if err != nil {
				return err
			}
			return mcp.ServeStdio(cmd.Context(), h)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "init [folder]",
		Short: "Create the data folder; the first run does this too",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 1 {
				homeFlag = args[0]
			}
			h, err := open()
			if err != nil {
				return err
			}
			fmt.Printf("kan-do data folder ready at %s\n", h.Dir)
			return nil
		},
	})
	return cmd
}
