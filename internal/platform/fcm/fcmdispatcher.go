// --- File: internal/platform/fcm/fcmdispatcher.go ---
// Package fcm will contain the client for Firebase Cloud Messaging.
package fcm

import (
	"context"
	"log/slog"

	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// LoggingDispatcher is a placeholder implementation of the notification.Dispatcher interface.
// It logs the notification details instead of sending them to a real service.
type LoggingDispatcher struct {
	logger *slog.Logger
}

// NewLoggingDispatcher creates a new logging-only dispatcher.
func NewLoggingDispatcher(logger *slog.Logger) (dispatch.Dispatcher, error) {
	dispatcher := &LoggingDispatcher{
		logger: logger.With("component", "FCMDispatcher"),
	}
	return dispatcher, nil
}

// Dispatch implements the notification.Dispatcher interface.
func (d *LoggingDispatcher) Dispatch(
	ctx context.Context,
	tokens []string,
	content notification.NotificationContent,
	data map[string]string,
) error {
	d.logger.Info("Dispatching notification.",
		"token_count", len(tokens),
		"title", content.Title,
		"body", content.Body,
		"data", data,
	)

	// In a real implementation, this is where the call to the FCM service would be.
	// For the MVP, we just return nil to simulate a successful dispatch.
	return nil
}
