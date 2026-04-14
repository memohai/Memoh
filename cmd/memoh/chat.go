package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/memohai/memoh/internal/tui"
)

func newChatCommand(ctx *cliContext) *cobra.Command {
	var botID string
	var sessionID string
	var message string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Send one chat message and stream the reply",
		RunE: func(_ *cobra.Command, _ []string) error {
			client := tui.NewClient(ctx.state.ServerURL, ctx.state.Token)
			if sessionID == "" {
				requestCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				sess, err := client.CreateSession(requestCtx, botID, message)
				if err != nil {
					return err
				}
				sessionID = sess.ID
				fmt.Printf("session: %s\n", sessionID)
			}

			streamCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			return client.StreamChat(streamCtx, tui.ChatRequest{
				BotID:     botID,
				SessionID: sessionID,
				Text:      message,
			}, func(event tui.ChatEvent) error {
				switch event.Type {
				case "start":
					fmt.Println("[start]")
				case "message":
					fmt.Println(tui.RenderUIMessage(event.Data))
				case "error":
					fmt.Println("[error]", event.Message)
				case "end":
					fmt.Println("[end]")
				}
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&botID, "bot", "", "Target bot ID")
	cmd.Flags().StringVar(&sessionID, "session", "", "Existing session ID")
	cmd.Flags().StringVar(&message, "message", "", "User message text")
	_ = cmd.MarkFlagRequired("bot")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}
