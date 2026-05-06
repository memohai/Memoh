package rbac

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

const (
	defaultCacheSize = 10000
	defaultCacheTTL  = 30 * time.Second
)

type Store interface {
	HasPermission(ctx context.Context, check Check) (bool, error)
	HasSystemAdmin(ctx context.Context, userID string) (bool, error)
}

type Service struct {
	store Store
	cache *expirable.LRU[string, bool]
}

func NewService(store Store) *Service {
	return NewServiceWithCache(store, defaultCacheSize, defaultCacheTTL)
}

func NewServiceWithCache(store Store, cacheSize int, ttl time.Duration) *Service {
	if cacheSize <= 0 {
		cacheSize = defaultCacheSize
	}
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	return &Service{
		store: store,
		cache: expirable.NewLRU[string, bool](cacheSize, nil, ttl),
	}
}

func (s *Service) HasPermission(ctx context.Context, check Check) (bool, error) {
	if s == nil || s.store == nil {
		return false, errors.New("rbac store is required")
	}
	if err := validateCheck(check); err != nil {
		return false, err
	}

	key := cacheKey(check)
	if allowed, ok := s.cache.Get(key); ok {
		return allowed, nil
	}

	allowed, err := s.evaluate(ctx, check)
	if err != nil {
		return false, err
	}
	s.cache.Add(key, allowed)
	return allowed, nil
}

func (s *Service) evaluate(ctx context.Context, check Check) (bool, error) {
	if check.PermissionKey != PermissionSystemAdmin {
		admin, err := s.store.HasSystemAdmin(ctx, check.UserID)
		if err != nil {
			return false, err
		}
		if admin {
			return true, nil
		}
	}
	return s.store.HasPermission(ctx, check)
}

func (s *Service) ClearCache() {
	if s == nil || s.cache == nil {
		return
	}
	s.cache.Purge()
}

func validateCheck(check Check) error {
	if strings.TrimSpace(check.UserID) == "" {
		return errors.New("user id is required")
	}
	if strings.TrimSpace(string(check.PermissionKey)) == "" {
		return errors.New("permission key is required")
	}
	if strings.TrimSpace(string(check.ResourceType)) == "" {
		return errors.New("resource type is required")
	}
	switch check.ResourceType {
	case ResourceSystem:
		if strings.TrimSpace(check.ResourceID) != "" {
			return errors.New("system resource id must be empty")
		}
	case ResourceBot:
	default:
		return fmt.Errorf("unsupported resource type %q", check.ResourceType)
	}
	return nil
}

func cacheKey(check Check) string {
	return strings.Join([]string{
		check.UserID,
		string(check.PermissionKey),
		string(check.ResourceType),
		check.ResourceID,
	}, "\x00")
}
