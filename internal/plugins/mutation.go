package plugins

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type botMutationLocker interface {
	WithBotMutationLock(context.Context, pgtype.UUID, func(dbstore.Queries) error) error
}

type transactionalQueries interface {
	InTx(context.Context, func(dbstore.Queries) error) error
}

type botMutationScopeKey struct{}

type botMutationScope struct {
	botID   string
	owner   *Service
	service *Service
}

var errCrossBotMutation = errors.New("cross-bot mutation nesting is not supported")

type localBotMutationLock struct {
	token chan struct{}
	refs  int
}

var localBotMutationLocks = struct {
	sync.Mutex
	items map[string]*localBotMutationLock
}{items: make(map[string]*localBotMutationLock)}

// WithBotMutation serializes Plugin and Registry Skill ownership changes for a
// bot. PostgreSQL-backed services also take a cross-process advisory lock.
func (s *Service) WithBotMutation(ctx context.Context, botID string, fn func(context.Context) error) error {
	if s == nil || s.queries == nil {
		return errors.New("plugin service is not configured")
	}
	if fn == nil {
		return errors.New("bot mutation callback is required")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	canonicalBotID := botUUID.String()
	if scope := ownedBotMutationScope(ctx, s); scope != nil {
		if scope.botID != canonicalBotID {
			return errCrossBotMutation
		}
		return fn(ctx)
	}

	release, err := acquireLocalBotMutationLock(ctx, canonicalBotID)
	if err != nil {
		return err
	}
	defer release()

	run := func(queries dbstore.Queries) error {
		scopedService := s.withQueries(queries)
		scopedCtx := context.WithValue(ctx, botMutationScopeKey{}, &botMutationScope{
			botID:   canonicalBotID,
			owner:   s,
			service: scopedService,
		})
		return fn(scopedCtx)
	}
	if locker, ok := s.queries.(botMutationLocker); ok {
		return locker.WithBotMutationLock(ctx, botUUID, run)
	}
	return run(s.queries)
}

func (s *Service) withBotMutation(
	ctx context.Context,
	botID string,
	fn func(context.Context, *Service) error,
) error {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	canonicalBotID := botUUID.String()
	if scope := botMutationScopeFromContext(ctx, s, canonicalBotID); scope != nil {
		return fn(ctx, scope.service)
	}
	return s.WithBotMutation(ctx, canonicalBotID, func(scopedCtx context.Context) error {
		scope := botMutationScopeFromContext(scopedCtx, s, canonicalBotID)
		if scope == nil {
			return errors.New("bot mutation scope was not established")
		}
		return fn(scopedCtx, scope.service)
	})
}

func botMutationScopeFromContext(ctx context.Context, service *Service, botID string) *botMutationScope {
	scope := ownedBotMutationScope(ctx, service)
	if scope == nil || scope.botID != botID {
		return nil
	}
	return scope
}

func ownedBotMutationScope(ctx context.Context, service *Service) *botMutationScope {
	scope, _ := ctx.Value(botMutationScopeKey{}).(*botMutationScope)
	if scope == nil {
		return nil
	}
	if scope.owner != service && scope.service != service {
		return nil
	}
	return scope
}

func (s *Service) scopedService(ctx context.Context, botID string) *Service {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil
	}
	scope := botMutationScopeFromContext(ctx, s, botUUID.String())
	if scope == nil {
		return nil
	}
	return scope.service
}

func (s *Service) inTransaction(ctx context.Context, fn func(*Service) error) error {
	tx, ok := s.queries.(transactionalQueries)
	if !ok {
		return fn(s)
	}
	return tx.InTx(ctx, func(queries dbstore.Queries) error {
		return fn(s.withQueries(queries))
	})
}

func (s *Service) withQueries(queries dbstore.Queries) *Service {
	clone := *s
	clone.queries = queries
	clone.mcpService = s.mcpService.WithQueries(queries)
	clone.oauthService = s.oauthService.WithQueries(queries)
	return &clone
}

func acquireLocalBotMutationLock(ctx context.Context, botID string) (func(), error) {
	localBotMutationLocks.Lock()
	item := localBotMutationLocks.items[botID]
	if item == nil {
		item = &localBotMutationLock{token: make(chan struct{}, 1)}
		item.token <- struct{}{}
		localBotMutationLocks.items[botID] = item
	}
	item.refs++
	localBotMutationLocks.Unlock()

	select {
	case <-ctx.Done():
		releaseLocalBotMutationLockRef(botID, item)
		return nil, ctx.Err()
	case <-item.token:
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			item.token <- struct{}{}
			releaseLocalBotMutationLockRef(botID, item)
		})
	}, nil
}

func releaseLocalBotMutationLockRef(botID string, item *localBotMutationLock) {
	localBotMutationLocks.Lock()
	defer localBotMutationLocks.Unlock()
	item.refs--
	if item.refs == 0 && localBotMutationLocks.items[botID] == item {
		delete(localBotMutationLocks.items, botID)
	}
}
