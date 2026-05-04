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
		Short: "Manage bots",
	}

	cmd.AddCommand(newBotsCreateCommand(ctx))
	cmd.AddCommand(newBotsDeleteCommand(ctx))

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
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := localClient(cmd.Context(), ctx)
			if err != nil {
				return err
			}

			requestCtx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			botID := strings.TrimSpace(args[0])
			if botID == "" {
				return errors.New("bot id is required")
			}
			if !yes {
				return fmt.Errorf("refusing to delete bot %s without --yes", botID)
			}

			client, err := localClient(cmd.Context(), ctx)
			if err != nil {
				return err
			}

			requestCtx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
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

// localClient returns a tui.Client targeting the desktop-managed local
// server. When --server is supplied it falls back to a token-less
// remote client (advanced override; assumes the caller knows the
// remote auth flow).
func localClient(ctx context.Context, cli *cliContext) (*tui.Client, error) {
	if cli.server != "" {
		return tui.NewClient(cli.state.ServerURL, ""), nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return tui.NewLocalClient(requestCtx)
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
