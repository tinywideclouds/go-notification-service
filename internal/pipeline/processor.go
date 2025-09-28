// Package pipeline contains the core message processing components for the service.
package pipeline

import (
	"context"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/illmade-knight/go-notification-service/pkg/notification"
	"github.com/illmade-knight/go-secure-messaging/pkg/transport"
	"github.com/rs/zerolog"
)

// NewProcessor is a constructor that creates the main message handler.
// It takes a map of dispatchers, allowing it to route notifications to the correct
// platform-specific client (e.g., APNS, FCM).
func NewProcessor(
	dispatchers map[string]notification.Dispatcher,
	logger zerolog.Logger,
) messagepipeline.StreamProcessor[transport.NotificationRequest] {

	// The returned function is the StreamProcessor that will be executed by the pipeline.
	return func(ctx context.Context, original messagepipeline.Message, request *transport.NotificationRequest) error {
		procLogger := logger.With().
			Str("recipient_id", request.RecipientID.String()).
			Str("pubsub_msg_id", original.ID).
			Logger()

		// 1. Group tokens by their platform.
		tokensByPlatform := make(map[string][]string)
		for _, token := range request.Tokens {
			tokensByPlatform[token.Platform] = append(tokensByPlatform[token.Platform], token.Token)
		}

		// 2. For each platform, find the correct dispatcher and send the notification.
		for platform, tokens := range tokensByPlatform {
			dispatcher, ok := dispatchers[platform]
			if !ok {
				procLogger.Warn().Str("platform", platform).Msg("No dispatcher configured for platform; skipping.")
				continue
			}

			procLogger.Info().Str("platform", platform).Int("token_count", len(tokens)).Msg("Dispatching notification.")
			err := dispatcher.Dispatch(ctx, tokens, request.Content, request.DataPayload)
			if err != nil {
				// If any dispatch fails, we return the error immediately.
				// The entire Pub/Sub message will be Nacked and retried.
				procLogger.Error().Err(err).Str("platform", platform).Msg("Dispatcher failed.")
				return err
			}
		}

		return nil // Success
	}
}
