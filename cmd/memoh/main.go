package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "memoh",
		Short: "Memoh unified binary",
		RunE: func(_ *cobra.Command, _ []string) error {
			runServe()
			return nil
		},
	}

	rootCmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Start the server",
		RunE: func(_ *cobra.Command, _ []string) error {
			runServe()
			return nil
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "migrate <up|down|version|force N>",
		Short: "Run database migrations",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runMigrate(args)
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runVersion()
		},
	})

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
