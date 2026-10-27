// Command kan-do is a kanban board for you and your coding agents: the board in your
// browser, and an MCP server agents use to work on the same tasks.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/azmainadel/kan-do/internal/export"
	"github.com/azmainadel/kan-do/internal/gitsync"
	"github.com/azmainadel/kan-do/internal/home"
	"github.com/azmainadel/kan-do/internal/mcp"
	"github.com/azmainadel/kan-do/internal/service"
	"github.com/azmainadel/kan-do/internal/update"
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
	var (
		repo       string
		keepLocal  bool
		keepRemote bool
	)
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Commit the data folder and pull or push it with its git remote",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			h, err := open()
			if err != nil {
				return err
			}
			if repo != "" {
				url, err := gitsync.SetRemote(h, repo, true)
				if err != nil {
					return err
				}
				fmt.Printf("The board syncs with %s.\n", url)
			}
			keep := gitsync.KeepNone
			if keepLocal {
				keep = gitsync.KeepLocal
			} else if keepRemote {
				keep = gitsync.KeepRemote
			}
			msg, err := gitsync.Sync(h, keep, true)
			if err != nil {
				return err
			}
			fmt.Println(msg)
			return nil
		},
	}
	syncCmd.Flags().StringVar(&repo, "repo", "", "sync with this git repo from now on, like https://github.com/you/kan-do-data or you/kan-do-data")
	syncCmd.Flags().BoolVar(&keepLocal, "keep-local", false, "when both sides changed, keep this machine's board and overwrite the remote")
	syncCmd.Flags().BoolVar(&keepRemote, "keep-remote", false, "when both sides changed, take the remote board and drop this machine's changes")
	syncCmd.MarkFlagsMutuallyExclusive("keep-local", "keep-remote")
	cmd.AddCommand(syncCmd)

	var format, project, output string
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Write the whole board as Markdown or JSON",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			h, err := open()
			if err != nil {
				return err
			}
			return export.Run(h, format, project, output)
		},
	}
	exportCmd.Flags().StringVarP(&format, "format", "f", "markdown", "output format: markdown or json")
	exportCmd.Flags().StringVarP(&project, "project", "p", "", "only this project")
	exportCmd.Flags().StringVarP(&output, "output", "o", "", "write to this file instead of the terminal")
	cmd.AddCommand(exportCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Install the latest release over this kan-do, and restart the board's service",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			latest := update.Latest(true)
			if latest == "" {
				return errors.New("can't reach GitHub to check for updates")
			}
			if !update.Newer(latest) {
				fmt.Printf("kan-do is up to date (%s).\n", version.Version)
				return nil
			}
			fmt.Printf("Installing kan-do %s (you have %s)…\n", latest, version.Version)
			if _, err := update.Install(); err != nil {
				return err
			}
			restarted, err := service.Restart()
			if err != nil {
				return err
			}
			if restarted {
				fmt.Printf("Updated to %s, and restarted the board.\n", latest)
			} else {
				fmt.Printf("Updated to %s.\n", latest)
			}
			return nil
		},
	})

	serviceCmd := &cobra.Command{
		Use:   "service",
		Short: "Start the board automatically at login (macOS and Linux)",
	}
	serviceCmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install and start a service that runs the board at login (LaunchAgent on macOS, systemd on Linux)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			h, err := open()
			if err != nil {
				return err
			}
			return service.Install(h)
		},
	}, &cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove that service",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return service.Uninstall() },
	})
	cmd.AddCommand(serviceCmd)
	return cmd
}
