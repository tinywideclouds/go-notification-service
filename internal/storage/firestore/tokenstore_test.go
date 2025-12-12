// --- File: internal/storage/firestore/tokenstore_test.go ---
//go:build integration

package firestore_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/illmade-knight/go-test/emulators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fsStore "github.com/tinywideclouds/go-notification-service/internal/storage/firestore"
	"github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func setupSuite(t *testing.T) (context.Context, *firestore.Client, *fsStore.TokenStore) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	projectID := "test-token-store"
	conn := emulators.SetupFirestoreEmulator(t, ctx, emulators.GetDefaultFirestoreConfig(projectID))
	client, err := firestore.NewClient(ctx, projectID, conn.ClientOptions...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	store := fsStore.NewTokenStore(client, "device-tokens", newTestLogger())
	return ctx, client, store
}

func TestTokenStore_Integration(t *testing.T) {
	ctx, _, store := setupSuite(t)
	userURN, _ := urn.Parse("urn:sm:user:test-user")

	t.Run("Register and Retrieve Tokens", func(t *testing.T) {
		// 1. Register a few tokens
		token1 := notification.DeviceToken{Token: "token-android-1", Platform: "android"}
		token2 := notification.DeviceToken{Token: "token-ios-1", Platform: "ios"}

		err := store.RegisterToken(ctx, userURN, token1)
		require.NoError(t, err)

		err = store.RegisterToken(ctx, userURN, token2)
		require.NoError(t, err)

		// 2. Retrieve them
		tokens, err := store.GetTokens(ctx, userURN)
		require.NoError(t, err)

		// 3. Verify
		assert.Len(t, tokens, 2)
		assert.Contains(t, tokens, token1)
		assert.Contains(t, tokens, token2)
	})

	t.Run("Idempotency (Upsert)", func(t *testing.T) {
		// Register the same token again
		token1 := notification.DeviceToken{Token: "token-android-1", Platform: "android"}
		err := store.RegisterToken(ctx, userURN, token1)
		require.NoError(t, err)

		// Should still only have 2 unique tokens (by ID)
		tokens, err := store.GetTokens(ctx, userURN)
		require.NoError(t, err)
		assert.Len(t, tokens, 2)
	})

	t.Run("Empty for unknown user", func(t *testing.T) {
		unknownURN, _ := urn.Parse("urn:sm:user:unknown")
		tokens, err := store.GetTokens(ctx, unknownURN)
		require.NoError(t, err)
		assert.Empty(t, tokens)
	})
}
