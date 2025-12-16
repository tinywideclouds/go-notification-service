// --- File: internal/platform/apns/apnsdispatcher.go ---
// Package apns provides the client for the Apple Push Notification Service.
package apns

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
	"github.com/sideshow/apns2/token"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// APNSClient defines the subset of the apns2.Client methods we use.
// This allows mocking for unit tests.
type APNSClient interface {
	Push(n *apns2.Notification) (*apns2.Response, error)
}

type Dispatcher struct {
	client APNSClient
	topic  string // The App Bundle ID (e.g. com.tinywide.messenger)
	logger *slog.Logger
}

// Config holds the credentials required to sign APNs tokens.
type Config struct {
	KeyID    string
	TeamID   string
	BundleID string
	// P8KeyContent is the raw string content of the .p8 file
	P8KeyContent string
}

// NewDispatcher creates a configured APNS dispatcher.
// It parses the P8 key immediately to fail fast on startup if credentials are bad.
func NewDispatcher(cfg Config, logger *slog.Logger) (*Dispatcher, error) {
	authKey, err := token.AuthKeyFromBytes([]byte(cfg.P8KeyContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse APNs P8 key: %w", err)
	}

	tokenSource := &token.Token{
		AuthKey: authKey,
		KeyID:   cfg.KeyID,
		TeamID:  cfg.TeamID,
	}

	// Use Production client by default.
	// In dev, the sandbox environment is usually determined by the device token itself
	// or separate certs, but for Token-based auth, Production endpoint is generally preferred
	// as it can route to sandbox if needed, though apns2.NewTokenClient defaults to Production.
	client := apns2.NewTokenClient(tokenSource)

	return &Dispatcher{
		client: client,
		topic:  cfg.BundleID,
		logger: logger.With("component", "APNSDispatcher"),
	}, nil
}

// Dispatch sends the notification to a batch of APNs tokens.
// Note: APNs HTTP/2 API is unary (one request per token). There is no "Multicast" endpoint.
// We iterate sequentially. For massive scale, this loop would be parallelized, but
// given this runs inside a scaled Pipeline Worker, serial processing per-user is acceptable.
func (d *Dispatcher) Dispatch(
	ctx context.Context,
	tokens []string,
	content notification.NotificationContent,
	data map[string]string,
) (string, []string, error) {
	if len(tokens) == 0 {
		return "skipped: no tokens", nil, nil
	}

	var invalidTokens []string
	successCount := 0
	failureCount := 0

	// 1. Build Payload
	// We use the builder pattern to construct the correct JSON structure
	builder := payload.NewPayload().
		AlertTitle(content.Title).
		AlertBody(content.Body).
		Sound(content.Sound)

	// Add custom data fields
	for k, v := range data {
		builder.Custom(k, v)
	}

	for _, deviceToken := range tokens {
		notification := &apns2.Notification{
			DeviceToken: deviceToken,
			Topic:       d.topic,
			Payload:     builder,
			// Expiration, Priority, etc. can be set here
		}

		// 2. Send (Synchronous HTTP/2)
		res, err := d.client.Push(notification)

		if err != nil {
			// Network/Transport Failure
			d.logger.Error("APNs transport failed", "token", deviceToken, "err", err)
			failureCount++
			continue
		}

		// 3. Handle Response Codes
		if res.Sent() {
			successCount++
		} else {
			failureCount++
			// Map APNs error reasons to our "Invalid" concept
			// See: https://developer.apple.com/documentation/usernotifications/setting_up_a_remote_notification_server/handling_notification_responses_from_apns
			switch res.Reason {
			case apns2.ReasonBadDeviceToken, apns2.ReasonUnregistered, apns2.ReasonDeviceTokenNotForTopic:
				// Token is dead. Add to cleanup list.
				invalidTokens = append(invalidTokens, deviceToken)
			default:
				// Other logic errors (TopicDisallowed, PayloadEmpty) are logged but not returned as "Invalid Token"
				// because the token might be fine, but our configuration is wrong.
				d.logger.Warn("APNs rejected notification", "reason", res.Reason, "status", res.StatusCode)
			}
		}
	}

	// If everything failed and it wasn't due to invalid tokens, we might want to signal a retry.
	// For now, we return the receipt.
	receipt := fmt.Sprintf("success:%d invalid:%d total_fail:%d", successCount, len(invalidTokens), failureCount)
	return receipt, invalidTokens, nil
}
