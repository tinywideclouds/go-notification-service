package firestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	urn "github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// FirestoreStore implements TokenStore using Google Cloud Firestore.
type FirestoreStore struct {
	client *firestore.Client
}

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client}
}

// deviceRecord is the internal DB representation.
// It can hold EITHER a simple string OR a complex object.
type deviceRecord struct {
	Platform        string                            `firestore:"platform"`
	Token           string                            `firestore:"token,omitempty"`            // Used for Android/iOS
	WebSubscription *notification.WebPushSubscription `firestore:"web_subscription,omitempty"` // Used for Web
	UpdatedAt       time.Time                         `firestore:"updated_at"`
}

// --- DOOR A: FCM (Mobile) ---

func (s *FirestoreStore) RegisterFCM(ctx context.Context, user urn.URN, token string) error {
	// Use hash of token as Doc ID to prevent duplicates and hot-spotting
	docID := hashToken(token)

	record := deviceRecord{
		Platform:  "fcm", // or "android"/"ios" passed in if you prefer specific tracking
		Token:     token,
		UpdatedAt: time.Now(),
	}

	_, err := s.deviceRef(user, docID).Set(ctx, record)
	return err
}

func (s *FirestoreStore) UnregisterFCM(ctx context.Context, user urn.URN, token string) error {
	docID := hashToken(token)
	_, err := s.deviceRef(user, docID).Delete(ctx)
	return err
}

// --- DOOR B: Web (VAPID) ---

func (s *FirestoreStore) RegisterWeb(ctx context.Context, user urn.URN, sub notification.WebPushSubscription) error {
	// For Web, the Endpoint URL is the unique identifier
	docID := hashToken(sub.Endpoint)

	record := deviceRecord{
		Platform:        "web",
		WebSubscription: &sub, // Store the full object
		UpdatedAt:       time.Now(),
	}

	_, err := s.deviceRef(user, docID).Set(ctx, record)
	return err
}

func (s *FirestoreStore) UnregisterWeb(ctx context.Context, user urn.URN, endpoint string) error {
	docID := hashToken(endpoint)
	_, err := s.deviceRef(user, docID).Delete(ctx)
	return err
}

// --- FAN-OUT (The Lookup) ---

func (s *FirestoreStore) Fetch(ctx context.Context, user urn.URN) (*notification.NotificationRequest, error) {
	// Query all devices for this user
	iter := s.devicesCollection(user).Documents(ctx)
	defer iter.Stop()

	// Initialize the buckets
	req := &notification.NotificationRequest{
		RecipientID:      user,
		FCMTokens:        make([]string, 0),
		WebSubscriptions: make([]notification.WebPushSubscription, 0),
	}

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("firestore iteration failed: %w", err)
		}

		var record deviceRecord
		if err := doc.DataTo(&record); err != nil {
			// Log and continue? Or fail? Usually safe to skip corrupt rows.
			continue
		}

		// SORTING HAT LOGIC
		if record.Platform == "web" && record.WebSubscription != nil {
			// Bucket B: Web
			req.WebSubscriptions = append(req.WebSubscriptions, *record.WebSubscription)
		} else if record.Token != "" {
			// Bucket A: Mobile (Default fallback)
			req.FCMTokens = append(req.FCMTokens, record.Token)
		}
	}

	return req, nil
}

// --- Helpers ---

// deviceRef: users/{userID}/devices/{deviceHash}
func (s *FirestoreStore) deviceRef(user urn.URN, docID string) *firestore.DocumentRef {
	return s.devicesCollection(user).Doc(docID)
}

func (s *FirestoreStore) devicesCollection(user urn.URN) *firestore.CollectionRef {
	// Assumes a root collection "users"
	return s.client.Collection("users").Doc(user.String()).Collection("devices")
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}
