package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/memohai/memoh/internal/tui"
)

type cliContext struct {
	state  tui.State
	server string
}

func newRootCommand() *cobra.Command {
	ctx := &cliContext{}

	rootCmd := &cobra.Command{
		Use:   "memoh",
		Short: "Memoh terminal operator CLI",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runTUI(ctx)
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

	rootCmd.PersistentFlags().StringVar(&ctx.server, "server", "", "Memoh server URL")

	rootCmd.AddCommand(newMigrateCommand())
	rootCmd.AddCommand(newInstallCommand())
	rootCmd.AddCommand(newLoginCommand(ctx))
	rootCmd.AddCommand(newChatCommand(ctx))
	rootCmd.AddCommand(newBotsCommand(ctx))
	rootCmd.AddCommand(newComposeCommands()...)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "tui",
		Short: "Open the terminal UI",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runTUI(ctx)
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runVersion()
		},
	})

	return rootCmd
}

func runTUI(ctx *cliContext) error {
	model := tui.NewTUIModel(ctx.state)
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}
