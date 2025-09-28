// Package apns will contain the client for the Apple Push Notification Service.
package apns

import (
	"context"

	"github.com/illmade-knight/go-notification-service/pkg/notification"
	"github.com/illmade-knight/go-secure-messaging/pkg/transport"
	"github.com/rs/zerolog"
)

// LoggingDispatcher is a placeholder implementation of the notification.Dispatcher interface.
// It logs the notification details instead of sending them to a real service.
type LoggingDispatcher struct {
	logger zerolog.Logger
}

// NewLoggingDispatcher creates a new logging-only dispatcher.
func NewLoggingDispatcher(logger zerolog.Logger) (notification.Dispatcher, error) {
	dispatcher := &LoggingDispatcher{
		logger: logger.With().Str("component", "APNSDispatcher").Logger(),
	}
	return dispatcher, nil
}

// Dispatch implements the notification.Dispatcher interface.
func (d *LoggingDispatcher) Dispatch(
	ctx context.Context,
	tokens []string,
	content transport.NotificationContent,
	data map[string]string,
) error {
	d.logger.Info().
		Int("token_count", len(tokens)).
		Str("title", content.Title).
		Str("body", content.Body).
		Interface("data", data).
		Msg("Dispatching notification.")

	// In a real implementation, this is where the call to the APNS would be.
	// For the MVP, we just return nil to simulate a successful dispatch.
	return nil
}
