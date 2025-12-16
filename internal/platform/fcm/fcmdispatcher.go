// --- File: internal/platform/fcm/fcmdispatcher.go ---
package fcm

import (
	"context"
	"fmt"
	"log/slog"

	"firebase.google.com/go/v4/messaging"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// MessagingClient defines the subset of the Firebase Messaging API we use.
// This interface allows us to mock the client for unit testing.
type MessagingClient interface {
	SendEachForMulticast(ctx context.Context, msg *messaging.MulticastMessage) (*messaging.BatchResponse, error)
}

type Dispatcher struct {
	client MessagingClient // Changed from *messaging.Client
	logger *slog.Logger
}

// NewDispatcher accepts the concrete client but stores it as the interface.
// Note: *messaging.Client automatically satisfies this interface.
func NewDispatcher(client MessagingClient, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		client: client,
		logger: logger.With("component", "FCMDispatcher"),
	}
}

// Dispatch remains exactly the same logic-wise...
func (d *Dispatcher) Dispatch(ctx context.Context, tokens []string, content notification.NotificationContent, data map[string]string) (string, []string, error) {
	if len(tokens) == 0 {
		return "skipped: no tokens", nil, nil
	}

	msg := &messaging.MulticastMessage{
		Tokens: tokens,
		Data:   data,
		Notification: &messaging.Notification{
			Title: content.Title,
			Body:  content.Body,
		},
		Webpush: &messaging.WebpushConfig{
			Notification: &messaging.WebpushNotification{
				Title: content.Title,
				Body:  content.Body,
				Icon:  "/assets/icons/icon-192x192.png",
			},
		},
	}

	// Uses the interface method
	br, err := d.client.SendEachForMulticast(ctx, msg)
	if err != nil {
		// ✅ CHECK: Is this a fatal validation error?
		// Note: The Firebase Go SDK returns standard error types.
		// We check if it's NOT a transport error.
		if messaging.IsInvalidArgument(err) {
			d.logger.Error("FCM rejected batch as InvalidArgument (dropping)", "err", err)
			// Return nil error to ACK the message and break the loop
			return "skipped: invalid_argument", nil, nil
		}

		// Real network/auth failure -> Retry
		return "", nil, fmt.Errorf("fcm transport failed: %w", err)
	}

	var invalidTokens []string
	retryableErrors := 0

	if br.FailureCount > 0 {
		for idx, resp := range br.Responses {
			if resp.Success {
				continue
			}

			// Check for FATAL errors (The token is garbage)
			if messaging.IsInvalidArgument(resp.Error) || messaging.IsRegistrationTokenNotRegistered(resp.Error) {
				invalidTokens = append(invalidTokens, tokens[idx])
				continue
			}

			retryableErrors++
		}
	}

	if retryableErrors > 0 {
		return "", nil, fmt.Errorf("batch had %d retryable errors", retryableErrors)
	}

	receipt := fmt.Sprintf("success:%d invalid:%d", br.SuccessCount, len(invalidTokens))
	return receipt, invalidTokens, nil
}
