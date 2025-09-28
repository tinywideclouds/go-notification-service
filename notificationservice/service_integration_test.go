//go:build integration

package notificationservice_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/google/uuid"
	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/illmade-knight/go-notification-service/notificationservice"
	"github.com/illmade-knight/go-notification-service/pkg/notification"
	"github.com/illmade-knight/go-secure-messaging/pkg/transport"
	"github.com/illmade-knight/go-test/emulators"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"
)

// mockDispatcher is a test double for the notification.Dispatcher interface.
type mockDispatcher struct {
	mu          sync.Mutex
	callCount   atomic.Int32
	failOnCount int32 // The call number to fail on. -1 means never fail.
	lastTokens  []string
	lastContent transport.NotificationContent
}

func newMockDispatcher(failOnCount int) *mockDispatcher {
	return &mockDispatcher{failOnCount: int32(failOnCount)}
}

func (m *mockDispatcher) Dispatch(ctx context.Context, tokens []string, content transport.NotificationContent, data map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentCall := m.callCount.Add(1)

	m.lastTokens = tokens
	m.lastContent = content

	if m.failOnCount > 0 && currentCall == m.failOnCount {
		return errors.New("dispatcher configured to fail")
	}

	return nil
}

func (m *mockDispatcher) GetCallCount() int {
	return int(m.callCount.Load())
}

func TestNotificationService_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	logger := zerolog.New(zerolog.NewTestWriter(t))
	projectID := "test-project"
	pubsubConn := emulators.SetupPubsubEmulator(t, ctx, emulators.GetDefaultPubsubConfig(projectID))
	psClient, err := pubsub.NewClient(ctx, projectID, pubsubConn.ClientOptions...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = psClient.Close() })

	t.Run("Success Path", func(t *testing.T) {
		// Arrange
		topicID := "push-notifications-success-" + uuid.NewString()
		subID := topicID + "-sub"
		createPubsubResources(t, ctx, psClient, projectID, topicID, subID)

		fcmDispatcher := newMockDispatcher(-1) // Never fails
		dispatchers := map[string]notification.Dispatcher{"android": fcmDispatcher}
		consumerCfg := *messagepipeline.NewGooglePubsubConsumerDefaults(subID)
		consumer, err := messagepipeline.NewGooglePubsubConsumer(&consumerCfg, psClient, logger)
		require.NoError(t, err)

		notificationService, err := notificationservice.New(":0", 2, consumer, dispatchers, logger)
		require.NoError(t, err)

		// Act
		serviceCtx, serviceCancel := context.WithCancel(ctx)
		defer serviceCancel()
		go func() {
			if err := notificationService.Start(serviceCtx); err != nil && !errors.Is(err, context.Canceled) {
				t.Logf("service.Start() returned an error: %v", err)
			}
		}()
		t.Cleanup(func() { _ = notificationService.Shutdown(context.Background()) })

		testProto := &transport.NotificationRequestPb{
			RecipientId: "urn:sm:user:integ-test-user",
			Tokens:      []*transport.DeviceTokenPb{{Token: "android-token-123", Platform: "android"}},
			Content:     &transport.NotificationRequestPbContent{Title: "Test Title"},
		}
		payload, err := protojson.Marshal(testProto)
		require.NoError(t, err)

		result := psClient.Publisher(topicID).Publish(ctx, &pubsub.Message{Data: payload})
		_, err = result.Get(ctx)
		require.NoError(t, err)

		// Assert
		require.Eventually(t, func() bool {
			return fcmDispatcher.GetCallCount() == 1
		}, 10*time.Second, 100*time.Millisecond, "Dispatcher was not called")

		fcmDispatcher.mu.Lock()
		defer fcmDispatcher.mu.Unlock()
		assert.Equal(t, []string{"android-token-123"}, fcmDispatcher.lastTokens)
		assert.Equal(t, "Test Title", fcmDispatcher.lastContent.Title)
		t.Log("✅ Dispatcher correctly called on success path.")
	})

	t.Run("Retry on Failure Path", func(t *testing.T) {
		// Arrange
		topicID := "push-notifications-retry-" + uuid.NewString()
		subID := topicID + "-sub"
		createPubsubResources(t, ctx, psClient, projectID, topicID, subID)

		fcmDispatcher := newMockDispatcher(1) // Fails on the first call
		dispatchers := map[string]notification.Dispatcher{"android": fcmDispatcher}

		// CORRECTED: Configure using the updated, embedded struct.
		consumerCfg := *messagepipeline.NewGooglePubsubConsumerDefaults(subID)
		consumerCfg.MaxOutstandingMessages = 1
		consumer, err := messagepipeline.NewGooglePubsubConsumer(&consumerCfg, psClient, logger)
		require.NoError(t, err)
		
		notificationService, err := notificationservice.New(":0", 2, consumer, dispatchers, logger)
		require.NoError(t, err)

		// Act
		serviceCtx, serviceCancel := context.WithCancel(ctx)
		defer serviceCancel()
		go func() {
			if err := notificationService.Start(serviceCtx); err != nil && !errors.Is(err, context.Canceled) {
				t.Logf("service.Start() returned an error: %v", err)
			}
		}()
		t.Cleanup(func() { _ = notificationService.Shutdown(context.Background()) })

		testProto := &transport.NotificationRequestPb{
			RecipientId: "urn:sm:user:retry-test-user",
			Tokens:      []*transport.DeviceTokenPb{{Token: "android-token-456", Platform: "android"}},
			Content:     &transport.NotificationRequestPbContent{Title: "Retry Title"},
		}
		payload, err := protojson.Marshal(testProto)
		require.NoError(t, err)
		result := psClient.Publisher(topicID).Publish(ctx, &pubsub.Message{Data: payload})
		_, err = result.Get(ctx)
		require.NoError(t, err)

		// Assert
		require.Eventually(t, func() bool {
			return fcmDispatcher.GetCallCount() >= 2
		}, 15*time.Second, 200*time.Millisecond, "Dispatcher was not called at least twice for retry")

		fcmDispatcher.mu.Lock()
		defer fcmDispatcher.mu.Unlock()
		assert.Equal(t, []string{"android-token-456"}, fcmDispatcher.lastTokens)
		assert.Equal(t, "Retry Title", fcmDispatcher.lastContent.Title)
		t.Log("✅ Dispatcher correctly retried and was called successfully.")
	})
}

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
