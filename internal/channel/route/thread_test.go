package route

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	session "github.com/memohai/memoh/internal/chat/thread"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type coordinatorRouteStore struct {
	route     Route
	getErr    error
	activeID  string
	activeErr error
}

func (s *coordinatorRouteStore) GetByID(context.Context, string) (Route, error) {
	return s.route, s.getErr
}

func (s *coordinatorRouteStore) SetActiveThread(_ context.Context, _ string, threadID string) error {
	s.activeID = threadID
	return s.activeErr
}

type coordinatorThreadStore struct {
	thread      session.Thread
	getID       string
	createInput session.CreateInput
	createErr   error
}

func (s *coordinatorThreadStore) Get(_ context.Context, id string) (session.Thread, error) {
	s.getID = id
	return s.thread, nil
}

func (s *coordinatorThreadStore) Create(_ context.Context, input session.CreateInput) (session.Thread, error) {
	s.createInput = input
	return s.thread, s.createErr
}

func TestThreadCoordinatorGetActiveReadsRouteThenThread(t *testing.T) {
	routes := &coordinatorRouteStore{route: Route{ActiveThreadID: "thread-1"}}
	threads := &coordinatorThreadStore{thread: session.Thread{ID: "thread-1"}}
	coordinator := NewThreadCoordinator(nil, routes, threads)

	got, err := coordinator.GetActive(context.Background(), "route-1")
	if err != nil {
		t.Fatalf("GetActive() error = %v", err)
	}
	if got.ID != "thread-1" || threads.getID != "thread-1" {
		t.Fatalf("GetActive() = %#v, get id = %q", got, threads.getID)
	}
}

func TestThreadCoordinatorGetActiveWithoutSelectionReturnsNoRows(t *testing.T) {
	coordinator := NewThreadCoordinator(nil, &coordinatorRouteStore{}, &coordinatorThreadStore{})
	_, err := coordinator.GetActive(context.Background(), "route-1")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("GetActive() error = %v, want pgx.ErrNoRows", err)
	}
}

func TestThreadCoordinatorCreateNewPreservesBestEffortActivation(t *testing.T) {
	routes := &coordinatorRouteStore{activeErr: errors.New("activation failed")}
	threads := &coordinatorThreadStore{thread: session.Thread{ID: "thread-1"}}
	coordinator := NewThreadCoordinator(nil, routes, threads)

	got, err := coordinator.CreateNew(context.Background(), session.CreateInput{
		BotID:   "bot-1",
		RouteID: "route-1",
	})
	if err != nil {
		t.Fatalf("CreateNew() error = %v", err)
	}
	if got.ID != "thread-1" {
		t.Fatalf("CreateNew() = %#v", got)
	}
	if threads.createInput.Type != session.TypeChat {
		t.Fatalf("created type = %q, want %q", threads.createInput.Type, session.TypeChat)
	}
	if routes.activeID != "thread-1" {
		t.Fatalf("activated thread = %q, want thread-1", routes.activeID)
	}
}

type routeThreadQueries struct {
	dbstore.Queries
	routes []sqlc.ListChatRoutesRow
}

func (q routeThreadQueries) ListChatRoutes(context.Context, pgtype.UUID) ([]sqlc.ListChatRoutesRow, error) {
	return q.routes, nil
}

func TestEnrichThreadsPreservesRouteProjection(t *testing.T) {
	routeID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	botID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	service := NewService(nil, routeThreadQueries{routes: []sqlc.ListChatRoutesRow{{
		ID:               routeID,
		BotID:            botID,
		Platform:         "telegram",
		ConversationType: pgtype.Text{String: "group", Valid: true},
		Metadata:         []byte(`{"conversation_name":"Memoh"}`),
	}}})

	got, err := service.EnrichThreads(context.Background(), botID.String(), []session.Thread{{
		ID:      "thread-1",
		RouteID: routeID.String(),
	}})
	if err != nil {
		t.Fatalf("EnrichThreads() error = %v", err)
	}
	if len(got) != 1 || got[0].RouteConversationType != "group" {
		t.Fatalf("EnrichThreads() = %#v", got)
	}
	if got[0].RouteMetadata["conversation_name"] != "Memoh" {
		t.Fatalf("RouteMetadata = %#v", got[0].RouteMetadata)
	}
}
