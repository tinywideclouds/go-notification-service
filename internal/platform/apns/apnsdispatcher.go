// --- File: internal/platform/apns/apnsdispatcher.go ---
// Package apns will contain the client for the Apple Push Notification Service.
package apns

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
		logger: logger.With("component", "APNSDispatcher"),
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

	// In a real implementation, this is where the call to the APNS would be.
	// For the MVP, we just return nil to simulate a successful dispatch.
	return nil
}
