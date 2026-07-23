package route

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	session "github.com/memohai/memoh/internal/chat/thread"
	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// ThreadStore is the Thread-owned persistence surface needed by Channel route
// orchestration.
type ThreadStore interface {
	Create(context.Context, session.CreateInput) (session.Thread, error)
	Get(context.Context, string) (session.Thread, error)
}

// ThreadRouteStore owns the active-thread pointer for an external route.
type ThreadRouteStore interface {
	GetByID(context.Context, string) (Route, error)
	SetActiveThread(context.Context, string, string) error
}

// ThreadCoordinator coordinates the Channel-owned route pointer with
// Thread-owned lifecycle operations.
type ThreadCoordinator struct {
	routes  ThreadRouteStore
	threads ThreadStore
	logger  *slog.Logger
}

func NewThreadCoordinator(log *slog.Logger, routes ThreadRouteStore, threads ThreadStore) *ThreadCoordinator {
	if log == nil {
		log = slog.Default()
	}
	return &ThreadCoordinator{
		routes:  routes,
		threads: threads,
		logger:  log.With(slog.String("service", "channel/route/thread")),
	}
}

// GetActive returns the active Thread selected by the route.
func (c *ThreadCoordinator) GetActive(ctx context.Context, routeID string) (session.Thread, error) {
	route, err := c.routes.GetByID(ctx, routeID)
	if err != nil {
		return session.Thread{}, err
	}
	if strings.TrimSpace(route.ActiveThreadID) == "" {
		return session.Thread{}, pgx.ErrNoRows
	}
	return c.threads.Get(ctx, route.ActiveThreadID)
}

// CreateNew creates a Thread and best-effort advances the route pointer. The
// historical behavior is preserved: a successful Thread creation is returned
// even when route activation fails.
func (c *ThreadCoordinator) CreateNew(ctx context.Context, input session.CreateInput) (session.Thread, error) {
	if strings.TrimSpace(input.Type) == "" {
		input.Type = session.TypeChat
	}
	thread, err := c.threads.Create(ctx, input)
	if err != nil {
		return session.Thread{}, fmt.Errorf("create new session: %w", err)
	}
	if err := c.routes.SetActiveThread(ctx, input.RouteID, thread.ID); err != nil {
		c.logger.Warn("failed to set active session on route", slog.Any("error", err))
	}
	return thread, nil
}

// EnsureActive returns the selected Thread, or creates one when the route
// currently has no usable active Thread.
func (c *ThreadCoordinator) EnsureActive(ctx context.Context, botID, routeID, channelType string) (session.Thread, error) {
	thread, err := c.GetActive(ctx, routeID)
	if err == nil {
		return thread, nil
	}
	thread, err = c.CreateNew(ctx, session.CreateInput{
		BotID:       botID,
		RouteID:     routeID,
		ChannelType: channelType,
	})
	if err != nil {
		return session.Thread{}, fmt.Errorf("auto-create session: %w", err)
	}
	return thread, nil
}

// SetActiveThread updates the Channel-owned active Thread pointer.
func (s *DBService) SetActiveThread(ctx context.Context, routeID, threadID string) error {
	pgRouteID, err := dbpkg.ParseUUID(routeID)
	if err != nil {
		return fmt.Errorf("invalid route id: %w", err)
	}
	var pgThreadID pgtype.UUID
	if strings.TrimSpace(threadID) != "" {
		pgThreadID, err = dbpkg.ParseUUID(threadID)
		if err != nil {
			return fmt.Errorf("invalid session id: %w", err)
		}
	}
	return s.queries.SetRouteActiveSession(ctx, sqlc.SetRouteActiveSessionParams{
		ID:              pgRouteID,
		ActiveSessionID: pgThreadID,
	})
}

// EnrichThreads projects Channel-owned route metadata onto Thread view
// records. It performs one route query for the bot and leaves unbound Threads
// unchanged.
func (s *DBService) EnrichThreads(ctx context.Context, botID string, threads []session.Thread) ([]session.Thread, error) {
	if len(threads) == 0 {
		return []session.Thread{}, nil
	}
	routes, err := s.List(ctx, botID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Route, len(routes))
	for _, route := range routes {
		byID[route.ID] = route
	}
	out := append([]session.Thread(nil), threads...)
	for i := range out {
		route, ok := byID[out[i].RouteID]
		if !ok {
			continue
		}
		out[i].RouteMetadata = route.Metadata
		out[i].RouteConversationType = route.ConversationType
	}
	return out, nil
}
