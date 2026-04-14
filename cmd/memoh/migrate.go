package main

import "github.com/spf13/cobra"

func newMigrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate <up|down|version|force N>",
		Short: "Run database migrations",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runMigrate(args)
		},
	}
}
