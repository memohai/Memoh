package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/tui"
)

func newBotsCommand(ctx *cliContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bots",
		Short: "Manage bots and their workspace containers",
	}

	cmd.AddCommand(newBotsCreateCommand(ctx))
	cmd.AddCommand(newBotsDeleteCommand(ctx))
	cmd.AddCommand(newBotsContainerCommand())

	return cmd
}

func newBotsCreateCommand(ctx *cliContext) *cobra.Command {
	var displayName string
	var avatarURL string
	var timezone string
	var inactive bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a bot",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			client, err := authenticatedClient(ctx)
			if err != nil {
				return err
			}

			requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			req := buildCreateBotRequest(displayName, avatarURL, timezone, inactive)

			bot, err := client.CreateBot(requestCtx, req)
			if err != nil {
				return err
			}

			fmt.Printf("Created bot %s (%s)\n", bot.DisplayName, bot.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&displayName, "name", "", "Bot display name")
	cmd.Flags().StringVar(&avatarURL, "avatar-url", "", "Bot avatar URL")
	cmd.Flags().StringVar(&timezone, "timezone", "", "Bot timezone")
	cmd.Flags().BoolVar(&inactive, "inactive", false, "Create the bot in inactive state")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func newBotsDeleteCommand(ctx *cliContext) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "delete <bot-id>",
		Short: "Delete a bot",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			botID := strings.TrimSpace(args[0])
			if botID == "" {
				return errors.New("bot id is required")
			}
			if !yes {
				return fmt.Errorf("refusing to delete bot %s without --yes", botID)
			}

			client, err := authenticatedClient(ctx)
			if err != nil {
				return err
			}

			requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := client.DeleteBot(requestCtx, botID); err != nil {
				return err
			}

			fmt.Printf("Deleted bot %s\n", botID)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm bot deletion")

	return cmd
}

func newBotsContainerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "ctr [ctr args]",
		Aliases:            []string{"container"},
		Short:              "Manage the nested containerd inside the server container",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		Long: `Run ctr inside the Docker Compose server service so you can inspect and
manage the nested containerd that Memoh uses for workspace containers.

By default this command injects the containerd namespace from config.toml.
Pass --no-namespace or provide your own ctr -n/--namespace flag to override it.`,
		Example: `  memoh bots ctr images ls
  memoh bots ctr containers ls
  memoh bots ctr --namespace default tasks ls
  memoh bots ctr --server-service server -- snapshots ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || isHelpArg(args[0]) {
				return cmd.Help()
			}

			opts, ctrArgs, err := parseContainerdOptions(args)
			if err != nil {
				return err
			}
			if len(ctrArgs) == 0 {
				return errors.New("missing ctr arguments")
			}

			return runServerContainerd(cmd.Context(), opts, ctrArgs)
		},
	}

	return cmd
}

func authenticatedClient(ctx *cliContext) (*tui.Client, error) {
	token := strings.TrimSpace(ctx.state.Token)
	if token == "" {
		return nil, errors.New("missing access token, please run `memoh login` first")
	}
	return tui.NewClient(ctx.state.ServerURL, token), nil
}

func buildCreateBotRequest(displayName, avatarURL, timezone string, inactive bool) bots.CreateBotRequest {
	req := bots.CreateBotRequest{
		DisplayName: strings.TrimSpace(displayName),
		AvatarURL:   strings.TrimSpace(avatarURL),
	}
	if strings.TrimSpace(timezone) != "" {
		tz := strings.TrimSpace(timezone)
		req.Timezone = &tz
	}
	if inactive {
		active := false
		req.IsActive = &active
	}
	return req
}
