package main

import (
	"github.com/spf13/cobra"
)

func newComposeCommands() []*cobra.Command {
	return []*cobra.Command{
		newComposeStartCommand(),
		newComposeStopCommand(),
		newComposeRestartCommand(),
		newComposeStatusCommand(),
		newComposeLogsCommand(),
		newComposeUpdateCommand(),
	}
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "help"
}

func addComposeFlags(cmd *cobra.Command, opts *dockerComposeOptions) {
	cmd.Flags().StringVar(&opts.projectDir, "project-dir", "", "Docker Compose project directory")
	cmd.Flags().StringSliceVarP(&opts.files, "file", "f", nil, "Additional compose file")
	cmd.Flags().StringSliceVar(&opts.profiles, "profile", nil, "Compose profile to enable")
	cmd.Flags().StringVar(&opts.envFile, "env-file", "", "Compose env file")
}

func newComposeStartCommand() *cobra.Command {
	opts := dockerComposeOptions{}
	var build bool

	cmd := &cobra.Command{
		Use:   "start [service...]",
		Short: "Start the Memoh stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			composeArgs := []string{"up", "-d"}
			if build {
				composeArgs = append(composeArgs, "--build")
			}
			composeArgs = append(composeArgs, args...)
			return runDockerCompose(cmd.Context(), opts, composeArgs)
		},
	}

	addComposeFlags(cmd, &opts)
	cmd.Flags().BoolVar(&build, "build", false, "Build images before starting")
	return cmd
}

func newComposeStopCommand() *cobra.Command {
	opts := dockerComposeOptions{}
	var volumes bool

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Memoh stack",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			composeArgs := []string{"down"}
			if volumes {
				composeArgs = append(composeArgs, "--volumes")
			}
			return runDockerCompose(cmd.Context(), opts, composeArgs)
		},
	}

	addComposeFlags(cmd, &opts)
	cmd.Flags().BoolVar(&volumes, "volumes", false, "Also remove named volumes")
	return cmd
}

func newComposeRestartCommand() *cobra.Command {
	opts := dockerComposeOptions{}
	cmd := &cobra.Command{
		Use:   "restart [service...]",
		Short: "Restart the Memoh stack or selected services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDockerCompose(cmd.Context(), opts, append([]string{"restart"}, args...))
		},
	}
	addComposeFlags(cmd, &opts)
	return cmd
}

func newComposeStatusCommand() *cobra.Command {
	opts := dockerComposeOptions{}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Memoh service status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDockerCompose(cmd.Context(), opts, []string{"ps"})
		},
	}
	addComposeFlags(cmd, &opts)
	return cmd
}

func newComposeLogsCommand() *cobra.Command {
	opts := dockerComposeOptions{}
	var follow bool
	var tail string

	cmd := &cobra.Command{
		Use:   "logs [service...]",
		Short: "Show Memoh service logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			composeArgs := []string{"logs"}
			if follow {
				composeArgs = append(composeArgs, "-f")
			}
			if tail != "" {
				composeArgs = append(composeArgs, "--tail", tail)
			}
			composeArgs = append(composeArgs, args...)
			return runDockerCompose(cmd.Context(), opts, composeArgs)
		},
	}

	addComposeFlags(cmd, &opts)
	cmd.Flags().BoolVar(&follow, "follow", false, "Follow log output")
	cmd.Flags().StringVar(&tail, "tail", "200", "Number of lines to show from the end of the logs")
	return cmd
}

func newComposeUpdateCommand() *cobra.Command {
	opts := dockerComposeOptions{}
	cmd := &cobra.Command{
		Use:   "update [service...]",
		Short: "Update the Memoh stack in one step",
		RunE: func(cmd *cobra.Command, args []string) error {
			pullArgs, upArgs := buildComposeUpdateSteps(args)
			if err := runDockerCompose(cmd.Context(), opts, pullArgs); err != nil {
				return err
			}
			return runDockerCompose(cmd.Context(), opts, upArgs)
		},
	}
	addComposeFlags(cmd, &opts)
	return cmd
}

func buildComposeUpdateSteps(services []string) ([]string, []string) {
	return append([]string{"pull"}, services...), append([]string{"up", "-d"}, services...)
}
