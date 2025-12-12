// --- File: pkg/dispatch/interfaces.go ---
package dispatch

import (
	"context"

	"github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// Dispatcher defines the contract for a component that can send notifications
// to a specific platform (e.g., Apple's APNS, Google's FCM).
type Dispatcher interface {
	// Dispatch sends the notification content to a batch of platform-specific tokens.
	Dispatch(ctx context.Context, tokens []string, content notification.NotificationContent, data map[string]string) error
}

// TokenStore defines the contract for managing user device tokens.
// It allows the service to remember "where" to send notifications for a user.
type TokenStore interface {
	// RegisterToken adds or updates a device token for a specific user.
	// It should handle deduplication (e.g., upsert).
	RegisterToken(ctx context.Context, userURN urn.URN, token notification.DeviceToken) error

	// GetTokens retrieves all active tokens for a specific user.
	GetTokens(ctx context.Context, userURN urn.URN) ([]notification.DeviceToken, error)
}
