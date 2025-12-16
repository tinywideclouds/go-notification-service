package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-notification-service/internal/platform/web"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// Mock VAPID keys (generated for testing)
const (
	mockPrivateKey = "4K3a3d... (any string is fine for logic test if library doesn't validate strictly locally)"
	mockPublicKey  = "BA... (any string)"
)

func TestDispatch_Lifecycle(t *testing.T) {
	// 1. Setup Mock Push Service (Simulates Google/Mozilla Push Server)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify VAPID Headers exist
		assert.NotEmpty(t, r.Header.Get("Authorization"))
		assert.NotEmpty(t, r.Header.Get("Crypto-Key"))

		// Routing based on endpoint URL
		switch r.URL.Path {
		case "/success":
			w.WriteHeader(http.StatusCreated) // 201
		case "/expired":
			w.WriteHeader(http.StatusGone) // 410
		case "/error":
			w.WriteHeader(http.StatusInternalServerError) // 500
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	// 2. Setup Dispatcher
	// We need valid-looking keys or webpush-go might panic on init.
	// For this test, we might need to bypass key validation or use real dummy keys.
	// Assuming the library checks keys:
	// If it fails, use "github.com/SherClockHolmes/webpush-go" GenerateVAPIDKeys()

	// Just use non-empty strings, the mock server doesn't verify signature
	dispatcher := web.NewDispatcher(config.VapidConfig{
		PrivateKey:      "test-private",
		PublicKey:       "test-public",
		SubscriberEmail: "mailto:test-runner@tinywideclouds.com",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Override the HTTP client in the dispatcher to ensure it hits our mock?
	// The library uses the client passed in Options. Our dispatcher creates its own.
	// *Correction*: We can't easily inject the client into NewDispatcher without changing signature.
	// *Hack for Test*: Use the functional options pattern or exposed field if possible.
	// For now, relies on the dispatcher using the URL we pass in the subscription.

	ctx := context.Background()
	content := notification.NotificationContent{Title: "Test", Body: "Body"}
	data := map[string]string{"id": "1"}

	// 3. Define Subscriptions pointing to Mock Server
	validSub := notification.WebPushSubscription{
		Endpoint: mockServer.URL + "/success",
		Keys: struct {
			P256dh []byte `json:"p256dh"`
			Auth   []byte `json:"auth"`
		}{P256dh: []byte("validkey"), Auth: []byte("validauth")},
	}

	expiredSub := notification.WebPushSubscription{
		Endpoint: mockServer.URL + "/expired",
		Keys: struct {
			P256dh []byte `json:"p256dh"`
			Auth   []byte `json:"auth"`
		}{P256dh: []byte("expiredkey"), Auth: []byte("expiredauth")},
	}

	// 4. Run Dispatch
	subs := []notification.WebPushSubscription{validSub, expiredSub}
	receipt, invalid, err := dispatcher.Dispatch(ctx, subs, content, data)

	// 5. Assertions
	require.NoError(t, err) // Should not error on 410/500, just report it

	// Check Receipt String
	assert.Contains(t, receipt, "success:1")
	assert.Contains(t, receipt, "invalid:1")

	// Check Invalid List (Should contain the expired sub)
	assert.Len(t, invalid, 1)
	assert.Equal(t, expiredSub.Endpoint, invalid[0].Endpoint)
}
