package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/memohai/memoh/internal/tui/local"
)

// newServiceCommands returns the lifecycle commands that operate on
// the desktop-managed local memoh-server process. They never reach for
// docker compose; everything is process-level (pid file + spawn).
func newServiceCommands(_ *cliContext) []*cobra.Command {
	return []*cobra.Command{
		newStartCommand(),
		newStopCommand(),
		newRestartCommand(),
		newStatusCommand(),
		newLogsCommand(),
	}
}

func newStartCommand() *cobra.Command {
	var waitReady bool
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the local Memoh server (requires desktop to have run once)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info, err := local.StartServer(local.SpawnOptions{})
			if err != nil {
				return err
			}
			fmt.Printf("Started memoh-server pid=%d\n", info.Pid)
			if !waitReady {
				return nil
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), waitTimeout)
			defer cancel()
			if err := local.WaitForReady(ctx, local.LocalServerBaseURL, waitTimeout); err != nil {
				return err
			}
			fmt.Printf("Server ready at %s\n", local.LocalServerBaseURL)
			return nil
		},
	}

	cmd.Flags().BoolVar(&waitReady, "wait", true, "Wait until the server responds on /ping before returning")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 30*time.Second, "How long to wait for readiness")
	return cmd
}

func newStopCommand() *cobra.Command {
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the local Memoh server",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			pidPath, err := local.PidPath()
			if err != nil {
				return err
			}
			killed, err := local.StopServer(pidPath, timeout)
			if err != nil {
				return err
			}
			if !killed {
				fmt.Println("No managed memoh-server process was running.")
				return nil
			}
			fmt.Println("Stopped memoh-server.")
			return nil
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Second, "Wait this long for graceful shutdown before SIGKILL")
	return cmd
}

func newRestartCommand() *cobra.Command {
	var timeout time.Duration
	var waitTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the local Memoh server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pidPath, err := local.PidPath()
			if err != nil {
				return err
			}
			if _, err := local.StopServer(pidPath, timeout); err != nil {
				return err
			}
			info, err := local.StartServer(local.SpawnOptions{})
			if err != nil {
				return err
			}
			fmt.Printf("Restarted memoh-server pid=%d\n", info.Pid)
			ctx, cancel := context.WithTimeout(cmd.Context(), waitTimeout)
			defer cancel()
			if err := local.WaitForReady(ctx, local.LocalServerBaseURL, waitTimeout); err != nil {
				return err
			}
			fmt.Printf("Server ready at %s\n", local.LocalServerBaseURL)
			return nil
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Second, "Wait this long for graceful shutdown before SIGKILL")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 30*time.Second, "How long to wait for readiness")
	return cmd
}

func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the status of the local Memoh server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pidPath, err := local.PidPath()
			if err != nil {
				return err
			}
			info, err := local.ReadPidFile(pidPath)
			if err != nil {
				return err
			}
			alive := info != nil && local.IsAlive(info.Pid)
			fmt.Printf("Endpoint:   %s\n", local.LocalServerBaseURL)
			if info == nil {
				fmt.Println("Pid file:   (none)")
			} else {
				fmt.Printf("Pid file:   pid=%d started=%s\n", info.Pid, info.StartedAt)
			}
			if !alive {
				fmt.Println("Process:    not running")
			} else {
				fmt.Println("Process:    alive")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
			defer cancel()
			status, err := local.Probe(ctx, local.LocalServerBaseURL)
			if err != nil {
				fmt.Printf("HTTP:       unreachable (%s)\n", err)
			} else {
				fmt.Printf("HTTP:       %s (version=%s commit=%s)\n", status.Status, status.Version, status.CommitHash)
			}
			return nil
		},
	}
	return cmd
}

func newLogsCommand() *cobra.Command {
	var follow bool
	var tail int

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show the local Memoh server log",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := local.LogPath()
			if err != nil {
				return err
			}
			return local.Tail(cmd.Context(), path, local.TailOptions{
				Tail:   tail,
				Follow: follow,
			}, os.Stdout)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output as new lines are appended")
	cmd.Flags().IntVarP(&tail, "tail", "n", 200, "Number of lines from the end to print before following")
	return cmd
}
