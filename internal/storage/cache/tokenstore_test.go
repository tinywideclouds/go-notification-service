// --- File: internal/storage/cache/tokenstore_test.go ---
package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-notification-service/internal/storage/cache"
	urn "github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// --- Mocks ---
type MockCache struct {
	mock.Mock
}

func (m *MockCache) Get(ctx context.Context, key string, dest interface{}) error {
	args := m.Called(ctx, key, dest)
	return args.Error(0)
}
func (m *MockCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return m.Called(ctx, key, value, ttl).Error(0)
}
func (m *MockCache) Del(ctx context.Context, key string) error {
	return m.Called(ctx, key).Error(0)
}

type MockRealStore struct {
	mock.Mock
}

// ... (Standard mock implementation for TokenStore, omitting boilerplate for brevity) ...
func (m *MockRealStore) UnregisterWeb(ctx context.Context, user urn.URN, endpoint string) error {
	return m.Called(ctx, user, endpoint).Error(0)
}
func (m *MockRealStore) Fetch(ctx context.Context, user urn.URN) (*notification.NotificationRequest, error) {
	args := m.Called(ctx, user)
	return args.Get(0).(*notification.NotificationRequest), args.Error(1)
}

// (Stub other methods as needed)
func (m *MockRealStore) RegisterFCM(context.Context, urn.URN, string) error { return nil }
func (m *MockRealStore) RegisterWeb(context.Context, urn.URN, notification.WebPushSubscription) error {
	return nil
}
func (m *MockRealStore) UnregisterFCM(context.Context, urn.URN, string) error { return nil }

func TestCachedStore_ImmediateInvalidation(t *testing.T) {
	ctx := context.Background()
	mockCache := new(MockCache)
	mockDB := new(MockRealStore)

	// Decorate the DB
	store := cache.NewCachedTokenStore(mockDB, mockCache, 1*time.Hour)
	userURN, _ := urn.Parse("urn:sm:user:annoyed-user")
	cacheKey := "notify:tokens:urn:sm:user:annoyed-user"

	t.Run("Unregister invalidates cache immediately", func(t *testing.T) {
		endpoint := "https://old.endpoint"

		// 1. Expect DB call
		mockDB.On("UnregisterWeb", ctx, userURN, endpoint).Return(nil)

		// 2. Expect Cache DELETE (Crucial!)
		mockCache.On("Del", ctx, cacheKey).Return(nil)

		// Act
		err := store.UnregisterWeb(ctx, userURN, endpoint)

		// Assert
		require.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockCache.AssertExpectations(t)
	})

	t.Run("Subsequent Fetch hits DB (Cache Miss)", func(t *testing.T) {
		// 1. Expect Cache Miss (simulating the delete worked)
		mockCache.On("Get", ctx, cacheKey, mock.Anything).Return(assert.AnError) // Error implies miss

		// 2. Expect DB Read (Source of Truth)
		// Return empty request (user disabled notifications)
		emptyReq := &notification.NotificationRequest{FCMTokens: []string{}}
		mockDB.On("Fetch", ctx, userURN).Return(emptyReq, nil)

		// 3. Expect Cache SET (Refilling with empty state)
		mockCache.On("Set", ctx, cacheKey, emptyReq, mock.Anything).Return(nil)

		// Act
		req, err := store.Fetch(ctx, userURN)

		// Assert
		require.NoError(t, err)
		require.Empty(t, req.FCMTokens)
		mockDB.AssertExpectations(t)
	})
}
