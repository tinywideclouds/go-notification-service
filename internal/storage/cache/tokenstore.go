// --- File: internal/storage/cache/tokenstore.go ---
package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	urn "github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// CacheClient defines the subset of Redis commands we need.
type CacheClient interface {
	// Get returns the value or a specific error if not found.
	Get(ctx context.Context, key string, dest interface{}) error
	// Set stores the value with a TTL.
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	// Del removes the key.
	Del(ctx context.Context, key string) error
}

// CachedTokenStore is a Decorator that adds Read-Aside caching to any TokenStore.
type CachedTokenStore struct {
	realStore dispatch.TokenStore
	cache     CacheClient
	ttl       time.Duration
}

// NewCachedTokenStore creates the decorator.
func NewCachedTokenStore(realStore dispatch.TokenStore, cache CacheClient, ttl time.Duration) *CachedTokenStore {
	return &CachedTokenStore{
		realStore: realStore,
		cache:     cache,
		ttl:       ttl,
	}
}

// --- READ PATH (Read-Aside) ---

func (s *CachedTokenStore) Fetch(ctx context.Context, user urn.URN) (*notification.NotificationRequest, error) {
	key := s.cacheKey(user)
	var cachedReq notification.NotificationRequest

	// 1. Try Cache
	// We only cache the lightweight Request struct (which contains the buckets), not the whole iterator logic.
	err := s.cache.Get(ctx, key, &cachedReq)
	if err == nil {
		// Cache Hit
		return &cachedReq, nil
	}

	// 2. Fallback to Real Store (Firestore)
	freshReq, err := s.realStore.Fetch(ctx, user)
	if err != nil {
		return nil, err
	}

	// 3. Populate Cache (Fire and Forget)
	// We ignore errors here because caching is an optimization, not a transaction.
	// If Redis is down, we just serve from DB.
	_ = s.cache.Set(ctx, key, freshReq, s.ttl)

	return freshReq, nil
}

// --- WRITE PATHS (Invalidate-on-Write) ---

func (s *CachedTokenStore) RegisterFCM(ctx context.Context, user urn.URN, token string) error {
	// 1. Write to Source of Truth
	if err := s.realStore.RegisterFCM(ctx, user, token); err != nil {
		return err
	}
	// 2. Invalidate Cache
	return s.invalidate(ctx, user)
}

func (s *CachedTokenStore) RegisterWeb(ctx context.Context, user urn.URN, sub notification.WebPushSubscription) error {
	if err := s.realStore.RegisterWeb(ctx, user, sub); err != nil {
		return err
	}
	return s.invalidate(ctx, user)
}

// UnregisterFCM handles the specific case you asked about.
// Even if the DB write succeeds, we MUST clear the cache to stop notifications immediately.
func (s *CachedTokenStore) UnregisterFCM(ctx context.Context, user urn.URN, token string) error {
	if err := s.realStore.UnregisterFCM(ctx, user, token); err != nil {
		return err
	}
	return s.invalidate(ctx, user)
}

func (s *CachedTokenStore) UnregisterWeb(ctx context.Context, user urn.URN, endpoint string) error {
	if err := s.realStore.UnregisterWeb(ctx, user, endpoint); err != nil {
		return err
	}
	return s.invalidate(ctx, user)
}

// --- Helpers ---

func (s *CachedTokenStore) invalidate(ctx context.Context, user urn.URN) error {
	// We delete the key. The next Fetch will be forced to go to Firestore.
	// This ensures immediate consistency for "Disable Notifications" actions.
	return s.cache.Del(ctx, s.cacheKey(user))
}

func (s *CachedTokenStore) cacheKey(user urn.URN) string {
	return fmt.Sprintf("notify:tokens:%s", user.String())
}
