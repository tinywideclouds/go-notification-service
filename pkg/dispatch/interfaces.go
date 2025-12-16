// --- File: pkg/dispatch/interfaces.go ---
package dispatch

import (
	"context"

	urn "github.com/tinywideclouds/go-platform/pkg/net/v1"
	notification "github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// Dispatcher defines the contract for platforms that use simple string tokens
// (e.g. Firebase Cloud Messaging for Android/iOS).
type Dispatcher interface {
	// Dispatch sends the notification to a list of string tokens.
	// Returns:
	// 1. Receipt string (log summary)
	// 2. []string: A list of FATALLY invalid tokens to be deleted.
	// 3. error: Only for RETRYABLE system failures.
	Dispatch(ctx context.Context, tokens []string, content notification.NotificationContent, data map[string]string) (string, []string, error)
}

// WebDispatcher defines the contract for platforms that use complex subscription objects
// (specifically W3C Web Push / VAPID).
type WebDispatcher interface {
	// Dispatch sends the notification to a list of WebPushSubscription objects.
	// Returns:
	// 1. Receipt string
	// 2. []WebPushSubscription: A list of FATALLY invalid subscriptions to be deleted (410 Gone).
	// 3. error: Retryable system failures.
	Dispatch(ctx context.Context, subs []notification.WebPushSubscription, content notification.NotificationContent, data map[string]string) (string, []notification.WebPushSubscription, error)
}

// TokenStore defines the storage contract for managing device registrations.
// It explicitly separates the "Mobile/String" path from the "Web/Object" path.
type TokenStore interface {
	// --- Registration (Write) ---
	RegisterFCM(ctx context.Context, user urn.URN, token string) error
	RegisterWeb(ctx context.Context, user urn.URN, sub notification.WebPushSubscription) error

	// --- Unregistration (Delete) ---
	UnregisterFCM(ctx context.Context, user urn.URN, token string) error
	UnregisterWeb(ctx context.Context, user urn.URN, endpoint string) error

	// --- Fan-Out (Read) ---
	// Fetch retrieves all devices for a user and populates the NotificationRequest
	// with the separated lists (FCMTokens and WebSubscriptions).
	Fetch(ctx context.Context, user urn.URN) (*notification.NotificationRequest, error)
}
