package pipeline

import (
	"context"
	"log/slog"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// NewProcessor creates the logic that handles the "Fan-Out".
// We inject specific dispatchers because the interfaces are now different (Strings vs Objects).
func NewProcessor(
	fcmDispatcher dispatch.Dispatcher, // Handles []string (Mobile)
	webDispatcher dispatch.WebDispatcher, // Handles []WebPushSubscription (Web)
	tokenStore dispatch.TokenStore,
	logger *slog.Logger,
) messagepipeline.StreamProcessor[notification.NotificationRequest] {

	return func(ctx context.Context, original messagepipeline.Message, request *notification.NotificationRequest) error {
		procLogger := logger.With(
			"recipient_id", request.RecipientID.String(),
			"pubsub_msg_id", original.ID,
		)

		// 1. Fetch & Fan-Out (The Lookup)
		// We re-fetch the request data from the store to populate the token buckets.
		// The incoming 'request' has the Content, but the Store has the Tokens.
		enrichedReq, err := tokenStore.Fetch(ctx, request.RecipientID)
		if err != nil {
			procLogger.Error("Failed to fetch device tokens", "err", err)
			return err
		}

		// 2. Path A: FCM (Mobile)
		if len(enrichedReq.FCMTokens) > 0 {
			receipt, invalidTokens, err := fcmDispatcher.Dispatch(ctx, enrichedReq.FCMTokens, request.Content, request.DataPayload)

			// Self-Healing (Strings)
			if len(invalidTokens) > 0 {
				procLogger.Info("Cleaning up invalid FCM tokens", "count", len(invalidTokens))
				for _, t := range invalidTokens {
					if err := tokenStore.UnregisterFCM(ctx, request.RecipientID, t); err != nil {
						procLogger.Warn("Failed to delete FCM token", "token", t, "err", err)
					}
				}
			}

			if err != nil {
				procLogger.Error("FCM Dispatch failed", "err", err)
				return err // Retryable
			}
			procLogger.Info("FCM Dispatched", "receipt", receipt)
		}

		// 3. Path B: Web (VAPID)
		if len(enrichedReq.WebSubscriptions) > 0 {
			receipt, invalidSubs, err := webDispatcher.Dispatch(ctx, enrichedReq.WebSubscriptions, request.Content, request.DataPayload)

			// Self-Healing (Objects - clean up by Endpoint)
			if len(invalidSubs) > 0 {
				procLogger.Info("Cleaning up invalid Web subscriptions", "count", len(invalidSubs))
				for _, sub := range invalidSubs {
					if err := tokenStore.UnregisterWeb(ctx, request.RecipientID, sub.Endpoint); err != nil {
						procLogger.Warn("Failed to delete Web subscription", "endpoint", sub.Endpoint, "err", err)
					}
				}
			}

			if err != nil {
				procLogger.Error("Web Dispatch failed", "err", err)
				return err // Retryable
			}
			procLogger.Info("Web Dispatched", "receipt", receipt)
		}

		if len(enrichedReq.FCMTokens) == 0 && len(enrichedReq.WebSubscriptions) == 0 {
			procLogger.Info("No devices registered for user; dropping notification.")
		}

		return nil
	}
}
