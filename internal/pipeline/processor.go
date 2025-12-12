// --- File: internal/pipeline/processor.go ---
package pipeline

import (
	"context"
	"log/slog"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// NewProcessor creates the main message handler.
// REFACTORED: Now accepts TokenStore to look up recipients.
func NewProcessor(
	dispatchers map[string]dispatch.Dispatcher,
	tokenStore dispatch.TokenStore, // <-- ADDED
	logger *slog.Logger,
) messagepipeline.StreamProcessor[notification.NotificationRequest] {

	return func(ctx context.Context, original messagepipeline.Message, request *notification.NotificationRequest) error {
		procLogger := logger.With(
			"recipient_id", request.RecipientID.String(),
			"pubsub_msg_id", original.ID,
		)

		// 1. Fetch Tokens (The new responsibility)
		// The request coming from Routing Service NO LONGER has tokens.
		// We must find them.
		tokens, err := tokenStore.GetTokens(ctx, request.RecipientID)
		if err != nil {
			procLogger.Error("Failed to fetch device tokens", "err", err)
			return err // Retryable error
		}

		if len(tokens) == 0 {
			procLogger.Info("No devices registered for user; dropping notification.")
			return nil
		}

		// 2. Group tokens by platform (Logic preserved)
		tokensByPlatform := make(map[string][]string)
		for _, token := range tokens {
			tokensByPlatform[token.Platform] = append(tokensByPlatform[token.Platform], token.Token)
		}

		// 3. Dispatch
		for platform, platformTokens := range tokensByPlatform {
			dispatcher, ok := dispatchers[platform]
			if !ok {
				procLogger.Warn("No dispatcher configured for platform", "platform", platform)
				continue
			}

			procLogger.Info("Dispatching notification", "platform", platform, "count", len(platformTokens))
			err := dispatcher.Dispatch(ctx, platformTokens, request.Content, request.DataPayload)
			if err != nil {
				procLogger.Error("Dispatcher failed", "err", err, "platform", platform)
				return err
			}
		}

		return nil
	}
}
