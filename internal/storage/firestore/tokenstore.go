// --- File: internal/storage/firestore/tokenstore.go ---
package firestore

import (
	"context"
	"fmt"
	"log/slog"

	"cloud.google.com/go/firestore"
	"github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// TokenDocument represents how we store tokens in Firestore.
// Structure: collection/userURN/devices/tokenString
type TokenDocument struct {
	Token     string      `firestore:"token"`
	Platform  string      `firestore:"platform"`
	UpdatedAt interface{} `firestore:"updated_at"` // ServerTimestamp
}

type TokenStore struct {
	client     *firestore.Client
	collection string
	logger     *slog.Logger
}

func NewTokenStore(client *firestore.Client, collectionName string, logger *slog.Logger) *TokenStore {
	return &TokenStore{
		client:     client,
		collection: collectionName,
		logger:     logger.With("component", "TokenStore"),
	}
}

func (s *TokenStore) RegisterToken(ctx context.Context, userURN urn.URN, token notification.DeviceToken) error {
	// Path: /device-tokens/{userURN}/devices/{token}
	// Using the token itself as the ID ensures easy deduplication.
	docRef := s.client.Collection(s.collection).
		Doc(userURN.String()).
		Collection("devices").
		Doc(token.Token) // Use token string as ID to prevent duplicates

	_, err := docRef.Set(ctx, map[string]interface{}{
		"token":      token.Token,
		"platform":   token.Platform,
		"updated_at": firestore.ServerTimestamp,
	})

	if err != nil {
		s.logger.Error("Failed to register token", "err", err, "user", userURN.String())
		return fmt.Errorf("failed to register token: %w", err)
	}

	s.logger.Info("Token registered successfully", "user", userURN.String(), "platform", token.Platform)
	return nil
}

func (s *TokenStore) GetTokens(ctx context.Context, userURN urn.URN) ([]notification.DeviceToken, error) {
	iter := s.client.Collection(s.collection).
		Doc(userURN.String()).
		Collection("devices").
		Documents(ctx)

	docs, err := iter.GetAll()
	if err != nil {
		s.logger.Error("Failed to fetch tokens", "err", err, "user", userURN.String())
		return nil, fmt.Errorf("failed to fetch tokens: %w", err)
	}

	var results []notification.DeviceToken
	for _, doc := range docs {
		var data TokenDocument
		if err := doc.DataTo(&data); err != nil {
			continue // Skip malformed docs
		}
		results = append(results, notification.DeviceToken{
			Token:    data.Token,
			Platform: data.Platform,
		})
	}

	return results, nil
}
