package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/memohai/memoh/internal/tui"
)

func newLoginCommand(ctx *cliContext) *cobra.Command {
	var username string
	var password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and persist a local access token",
		RunE: func(_ *cobra.Command, _ []string) error {
			client := tui.NewClient(ctx.state.ServerURL, "")
			requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := client.Login(requestCtx, username, password)
			if err != nil {
				return err
			}

			next := ctx.state
			next.ServerURL = client.BaseURL
			next.Token = resp.AccessToken
			next.Username = resp.Username
			if parsed, err := time.Parse(time.RFC3339, resp.ExpiresAt); err == nil {
				next.ExpiresAt = parsed
			}
			if err := tui.SaveState(next); err != nil {
				return err
			}

			fmt.Printf("Logged in as %s against %s\n", resp.Username, client.BaseURL)
			return nil
		},
	}

	cmd.Flags().StringVar(&username, "username", "", "Account username")
	cmd.Flags().StringVar(&password, "password", "", "Account password")
	_ = cmd.MarkFlagRequired("username")
	_ = cmd.MarkFlagRequired("password")
	return cmd
}
