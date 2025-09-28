// Package notification contains the public interfaces and domain models for the
// notification service.
package notification

import (
	"context"

	"github.com/illmade-knight/go-secure-messaging/pkg/transport"
)

// Dispatcher defines the contract for a component that can send notifications
// to a specific platform (e.g., Apple's APNS, Google's FCM).
type Dispatcher interface {
	// Dispatch sends the notification content to a batch of platform-specific tokens.
	Dispatch(ctx context.Context, tokens []string, content transport.NotificationContent, data map[string]string) error
}
