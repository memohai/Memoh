package main

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/memohai/memoh/internal/tui"
	"github.com/memohai/memoh/internal/version"
)

// cliContext is shared by every cobra subcommand. It carries the
// loaded user preferences (server URL, etc.) and CLI-level overrides.
type cliContext struct {
	state  tui.State
	server string
}

func newRootCommand() *cobra.Command {
	ctx := &cliContext{}

	rootCmd := &cobra.Command{
		Use:   "memoh",
		Short: "Memoh desktop companion CLI",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd.Context(), ctx)
		},
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			state, err := tui.LoadState()
			if err != nil {
				return err
			}
			ctx.state = state
			if ctx.server != "" {
				ctx.state.ServerURL = tui.NormalizeServerURL(ctx.server)
			}
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&ctx.server, "server", "", "Override the local Memoh server URL (defaults to "+tui.DefaultProdServerURL+")")

	rootCmd.AddCommand(newChatCommand(ctx))
	rootCmd.AddCommand(newBotsCommand(ctx))
	rootCmd.AddCommand(newServiceCommands(ctx)...)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "tui",
		Short: "Open the terminal UI",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd.Context(), ctx)
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Printf("memoh %s\n", version.GetInfo())
			return nil
		},
	})

	return rootCmd
}

func runTUI(ctx context.Context, cli *cliContext) error {
	client, err := localClient(ctx, cli)
	if err != nil {
		return err
	}
	model := tui.NewTUIModel(cli.state, client)
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}
