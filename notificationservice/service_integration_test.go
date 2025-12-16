// --- File: notificationservice/service_integration_test.go ---
//go:build integration

package notificationservice_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/google/uuid"
	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/illmade-knight/go-test/emulators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-notification-service/notificationservice"
	"github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
	"google.golang.org/protobuf/types/known/durationpb"

	fsStore "github.com/tinywideclouds/go-notification-service/internal/storage/firestore"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"
)

// --- MOCKS ---

// Mock for FCM (Strings)
type mockDispatcher struct {
	mu          sync.Mutex
	callCount   int
	lastTokens  []string
	failOnCount int
}

func newMockDispatcher(failOnCount int) *mockDispatcher {
	return &mockDispatcher{failOnCount: failOnCount}
}
func (m *mockDispatcher) Dispatch(ctx context.Context, tokens []string, content notification.NotificationContent, data map[string]string) (string, []string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	m.lastTokens = tokens
	if m.failOnCount > 0 && m.callCount == m.failOnCount {
		return "", nil, errors.New("fail")
	}
	return "123-343-success", nil, nil
}
func (m *mockDispatcher) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}
func (m *mockDispatcher) GetLastTokens() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastTokens
}

// Mock for Web (Objects) - Required by New()
type mockWebDispatcher struct {
	mu sync.Mutex
}

func (m *mockWebDispatcher) Dispatch(ctx context.Context, subs []notification.WebPushSubscription, content notification.NotificationContent, data map[string]string) (string, []notification.WebPushSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// No-op for this test, but must exist
	return "web-success", nil, nil
}

// --- TEST ---

func TestNotificationService_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	projectID := "test-project-integ"

	// 1. Emulators
	pubsubConn := emulators.SetupPubsubEmulator(t, ctx, emulators.GetDefaultPubsubConfig(projectID))
	psClient, err := pubsub.NewClient(ctx, projectID, pubsubConn.ClientOptions...)
	require.NoError(t, err)
	t.Cleanup(func() { psClient.Close() })

	fsConn := emulators.SetupFirestoreEmulator(t, ctx, emulators.GetDefaultFirestoreConfig(projectID))
	fsClient, err := firestore.NewClient(ctx, projectID, fsConn.ClientOptions...)
	require.NoError(t, err)
	t.Cleanup(func() { fsClient.Close() })

	// 2. Token Store (Firestore Implementation)
	tokenStore := fsStore.NewFirestoreStore(fsClient)

	t.Run("Full Lifecycle: Register -> Process -> Dispatch", func(t *testing.T) {
		// Arrange
		topicID := "push-success-" + uuid.NewString()
		subID := topicID + "-sub"
		createPubsubResources(t, ctx, psClient, projectID, topicID, subID)

		fcmDispatcher := newMockDispatcher(-1)
		webDispatcher := &mockWebDispatcher{}

		consumerCfg := *messagepipeline.NewGooglePubsubConsumerDefaults(subID)
		consumer, _ := messagepipeline.NewGooglePubsubConsumer(&consumerCfg, psClient, logger)

		// Create Service (Using updated signature)
		svc, err := notificationservice.New(
			&config.Config{ListenAddr: ":0", NumPipelineWorkers: 2}, // Mock Config
			consumer,
			fcmDispatcher, // Explicit FCM
			webDispatcher, // Explicit Web
			tokenStore,
			func(h http.Handler) http.Handler { return h }, // No-op Auth
			logger,
		)
		require.NoError(t, err)

		// Start Service
		svcCtx, svcCancel := context.WithCancel(ctx)
		defer svcCancel()
		go func() { svc.Start(svcCtx) }()
		t.Cleanup(func() { svc.Shutdown(context.Background()) })

		// Step A: Register Token (Using new RegisterFCM method)
		userURN, _ := urn.Parse("urn:sm:user:integ-user")
		err = tokenStore.RegisterFCM(ctx, userURN, "android-token-999")
		require.NoError(t, err)

		// Step B: Publish Message (WITHOUT TOKENS)
		// The service will fetch the "android-token-999" from Firestore
		req := &notification.NotificationRequest{
			RecipientID: userURN,
			Content:     notification.NotificationContent{Title: "Hello"},
		}
		payload, _ := json.Marshal(req)

		psClient.Publisher(topicID).Publish(ctx, &pubsub.Message{Data: payload}).Get(ctx)

		// Assert: FCM Dispatcher called with the token we registered in Step A
		require.Eventually(t, func() bool {
			return fcmDispatcher.GetCallCount() == 1
		}, 10*time.Second, 100*time.Millisecond)

		assert.Equal(t, []string{"android-token-999"}, fcmDispatcher.GetLastTokens())
	})
}

// ... (createPubsubResources helper remains unchanged) ...
func createPubsubResources(t *testing.T, ctx context.Context, client *pubsub.Client, projectID, topicID, subID string) {
	t.Helper()
	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	_, err := client.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.TopicAdminClient.DeleteTopic(context.Background(), &pubsubpb.DeleteTopicRequest{Topic: topicName})
	})

	subName := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subID)
	sub := &pubsubpb.Subscription{
		Name:               subName,
		Topic:              topicName,
		AckDeadlineSeconds: 10,
		RetryPolicy: &pubsubpb.RetryPolicy{
			MinimumBackoff: &durationpb.Duration{Seconds: 1},
		},
	}
	_, err = client.SubscriptionAdminClient.CreateSubscription(ctx, sub)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.SubscriptionAdminClient.DeleteSubscription(context.Background(), &pubsubpb.DeleteSubscriptionRequest{Subscription: subName})
	})
}
