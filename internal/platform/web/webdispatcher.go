package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

type Dispatcher struct {
	subscriber string
	privateKey string
	publicKey  string
	logger     *slog.Logger
	httpClient *http.Client
}

func NewDispatcher(cfg config.VapidConfig, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		privateKey: cfg.PrivateKey,
		publicKey:  cfg.PublicKey,
		subscriber: cfg.SubscriberEmail,
		logger:     logger.With("component", "WebPushDispatcher"),
		httpClient: &http.Client{},
	}
}

// Dispatch now accepts the strict []notification.WebPushSubscription slice.
// It returns a list of failed subscriptions that should be removed from the DB.
func (d *Dispatcher) Dispatch(
	ctx context.Context,
	subs []notification.WebPushSubscription,
	content notification.NotificationContent,
	data map[string]string,
) (string, []notification.WebPushSubscription, error) {

	var invalidSubs []notification.WebPushSubscription
	successCount := 0
	failureCount := 0

	// 1. Prepare Payload (Standard JSON structure)
	payloadBytes, err := json.Marshal(map[string]interface{}{
		"notification": map[string]string{
			"title": content.Title,
			"body":  content.Body,
			// Add icon/actions here if needed from content
		},
		"data": data,
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	for _, sub := range subs {
		// 2. Build the VAPID Subscription
		s := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				// ✅ Encode []byte -> Base64 String for the library
				P256dh: base64.RawURLEncoding.EncodeToString(sub.Keys.P256dh),
				Auth:   base64.RawURLEncoding.EncodeToString(sub.Keys.Auth),
			},
		}

		// 3. Send via webpush-go
		resp, err := webpush.SendNotification(payloadBytes, s, &webpush.Options{
			Subscriber:      d.subscriber,
			VAPIDPublicKey:  d.publicKey,
			VAPIDPrivateKey: d.privateKey,
			TTL:             60,
			HTTPClient:      d.httpClient,
		})

		if err != nil {
			// Transport error (DNS, Timeout) - Log and skip, don't delete
			d.logger.Error("WebPush transport error", "endpoint", sub.Endpoint, "err", err)
			failureCount++
			continue
		}
		defer resp.Body.Close()

		// 4. Handle Response Codes
		switch resp.StatusCode {
		case 201:
			successCount++
		case 410, 404:
			// 410 Gone / 404 Not Found -> Token is dead, return for cleanup
			invalidSubs = append(invalidSubs, sub)
			failureCount++
		default:
			d.logger.Warn("WebPush rejected", "status", resp.StatusCode, "endpoint", sub.Endpoint)
			failureCount++
		}
	}

	receipt := fmt.Sprintf("success:%d invalid:%d total_fail:%d", successCount, len(invalidSubs), failureCount)
	return receipt, invalidSubs, nil
}
