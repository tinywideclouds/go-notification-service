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

	fs "github.com/tinywideclouds/go-notification-service/internal/storage/firestore"

	urn "github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func setupSuite(t *testing.T) (context.Context, *firestore.Client, *fs.FirestoreStore) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	projectID := "test-token-store"
	conn := emulators.SetupFirestoreEmulator(t, ctx, emulators.GetDefaultFirestoreConfig(projectID))
	client, err := firestore.NewClient(ctx, projectID, conn.ClientOptions...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	// FIX: Use the constructor from the dispatch package
	store := fs.NewFirestoreStore(client)
	return ctx, client, store
}

func TestTokenStore_Integration(t *testing.T) {
	ctx, _, store := setupSuite(t)
	userURN, _ := urn.Parse("urn:contacts:user:test-user")

	t.Run("FCM Registration Lifecycle", func(t *testing.T) {
		// 1. Register FCM
		token := "token-android-1"
		err := store.RegisterFCM(ctx, userURN, token)
		require.NoError(t, err)

		// 2. Fetch and Verify
		req, err := store.Fetch(ctx, userURN)
		require.NoError(t, err)

		// Assert it landed in the FCM bucket
		assert.Len(t, req.FCMTokens, 1)
		assert.Contains(t, req.FCMTokens, token)
		assert.Empty(t, req.WebSubscriptions)

		// 3. Unregister
		err = store.UnregisterFCM(ctx, userURN, token)
		require.NoError(t, err)

		// 4. Verify Gone
		reqAfter, err := store.Fetch(ctx, userURN)
		require.NoError(t, err)
		assert.Empty(t, reqAfter.FCMTokens)
	})

	t.Run("Web Push Registration Lifecycle", func(t *testing.T) {
		// 1. Register Web
		sub := notification.WebPushSubscription{
			Endpoint: "https://fcm.googleapis.com/fcm/send/abc-123",
			Keys: struct {
				P256dh []byte `json:"p256dh"` // ✅ Updated type
				Auth   []byte `json:"auth"`   // ✅ Updated type
			}{
				P256dh: []byte{0xDE, 0xAD, 0xBE, 0xEF}, // ✅ Real binary data
				Auth:   []byte{0xCA, 0xFE, 0xBA, 0xBE},
			},
		}

		err := store.RegisterWeb(ctx, userURN, sub)
		require.NoError(t, err)

		// 2. Fetch and Verify
		req, err := store.Fetch(ctx, userURN)
		require.NoError(t, err)

		// Assert it landed in the Web bucket
		assert.Len(t, req.WebSubscriptions, 1)
		assert.Equal(t, sub.Endpoint, req.WebSubscriptions[0].Endpoint)
		assert.Empty(t, req.FCMTokens)

		// 3. Unregister (Web uses endpoint as key)
		err = store.UnregisterWeb(ctx, userURN, sub.Endpoint)
		require.NoError(t, err)

		// 4. Verify Gone
		_, err = store.Fetch(ctx, userURN)
		require.NoError(t, err)
		assert.Empty(t, req.WebSubscriptions)
	})

	t.Run("Fan-Out Fetch (Mixed Types)", func(t *testing.T) {
		// Setup: Register one of each
		fcmToken := "token-ios-mix"
		webSub := notification.WebPushSubscription{
			Endpoint: "https://web.push/mix",
			Keys: struct {
				P256dh []byte `json:"p256dh"`
				Auth   []byte `json:"auth"`
			}{
				P256dh: []byte{0xDE, 0xAD, 0xBE, 0xEF},
				Auth:   []byte{0xCA, 0xFE, 0xBA, 0xBE},
			},
		}

		require.NoError(t, store.RegisterFCM(ctx, userURN, fcmToken))
		require.NoError(t, store.RegisterWeb(ctx, userURN, webSub))

		// Act: Fetch
		req, err := store.Fetch(ctx, userURN)
		require.NoError(t, err)

		// Assert: The store correctly sorted the mixed DB records into buckets
		assert.Len(t, req.FCMTokens, 1)
		assert.Contains(t, req.FCMTokens, fcmToken)

		assert.Len(t, req.WebSubscriptions, 1)
		assert.Equal(t, webSub.Endpoint, req.WebSubscriptions[0].Endpoint)
	})
}
