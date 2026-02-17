// feishu-echo is a minimal Feishu bot that connects via WebSocket and counts received events.
// Used to verify whether message loss is due to our app logic or network/Feishu delivery.
//
// Usage:
//
//	FEISHU_APP_ID=xxx FEISHU_APP_SECRET=xxx FEISHU_ENCRYPT=xxx FEISHU_VERIFY=xxx go run ./cmd/feishu-echo
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

type eventCounts struct {
	messageReceive  atomic.Int64
	messageRead     atomic.Int64
	reactionCreated atomic.Int64
	reactionDeleted atomic.Int64
}

func (c *eventCounts) log() {
	log.Printf("[feishu-echo] counts: receive=%d read=%d reaction_created=%d reaction_deleted=%d",
		c.messageReceive.Load(), c.messageRead.Load(), c.reactionCreated.Load(), c.reactionDeleted.Load())
}

func main() {
	appID := strings.TrimSpace(os.Getenv("FEISHU_APP_ID"))
	appSecret := strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET"))
	encryptKey := strings.TrimSpace(os.Getenv("FEISHU_ENCRYPT"))
	verifyToken := strings.TrimSpace(os.Getenv("FEISHU_VERIFY"))

	if appID == "" || appSecret == "" {
		log.Fatal("FEISHU_APP_ID and FEISHU_APP_SECRET are required")
	}

	log.Printf("[feishu-echo] starting with app_id=%s (encrypt=%v, verify=%v)", appID, encryptKey != "", verifyToken != "")

	counts := new(eventCounts)
	eventDispatcher := dispatcher.NewEventDispatcher(verifyToken, encryptKey)

	eventDispatcher.OnP2MessageReceiveV1(func(_ context.Context, _ *larkim.P2MessageReceiveV1) error {
		counts.messageReceive.Add(1)
		counts.log()
		return nil
	})

	eventDispatcher.OnP2MessageReadV1(func(_ context.Context, _ *larkim.P2MessageReadV1) error {
		counts.messageRead.Add(1)
		counts.log()
		return nil
	})

	eventDispatcher.OnP2MessageReactionCreatedV1(func(_ context.Context, _ *larkim.P2MessageReactionCreatedV1) error {
		counts.reactionCreated.Add(1)
		counts.log()
		return nil
	})

	eventDispatcher.OnP2MessageReactionDeletedV1(func(_ context.Context, _ *larkim.P2MessageReactionDeletedV1) error {
		counts.reactionDeleted.Add(1)
		counts.log()
		return nil
	})

	client := larkws.NewClient(
		appID,
		appSecret,
		larkws.WithEventHandler(eventDispatcher),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		log.Println("[feishu-echo] interrupt, shutting down")
		cancel()
		counts.log()
		os.Exit(0)
	}()

	const reconnectDelay = 3 * time.Second
run:
	for {
		if ctx.Err() != nil {
			break run
		}
		log.Println("[feishu-echo] connecting to Feishu WebSocket...")
		err := client.Start(ctx)
		if ctx.Err() != nil {
			break run
		}
		if err != nil {
			log.Printf("[feishu-echo] client error: %v; reconnecting in %v", err, reconnectDelay)
		} else {
			log.Printf("[feishu-echo] connection closed; reconnecting in %v", reconnectDelay)
		}
		timer := time.NewTimer(reconnectDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			break run
		case <-timer.C:
		}
	}
	counts.log()
	log.Println("[feishu-echo] stopped")
}
