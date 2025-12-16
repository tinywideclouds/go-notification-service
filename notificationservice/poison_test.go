// --- File: notificationservice/poison_test.go ---
//go:build integration

package notificationservice_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/google/uuid"
	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/illmade-knight/go-test/emulators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-notification-service/notificationservice"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"
	"github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

// --- Mocks ---

// mockTokenStore updated to match pkg/dispatch/interfaces.go
type mockTokenStore struct {
	mock.Mock
}

func (m *mockTokenStore) RegisterFCM(ctx context.Context, userURN urn.URN, token string) error {
	return m.Called(ctx, userURN, token).Error(0)
}
func (m *mockTokenStore) RegisterWeb(ctx context.Context, userURN urn.URN, sub notification.WebPushSubscription) error {
	return m.Called(ctx, userURN, sub).Error(0)
}
func (m *mockTokenStore) UnregisterFCM(ctx context.Context, userURN urn.URN, token string) error {
	return m.Called(ctx, userURN, token).Error(0)
}
func (m *mockTokenStore) UnregisterWeb(ctx context.Context, userURN urn.URN, endpoint string) error {
	return m.Called(ctx, userURN, endpoint).Error(0)
}
func (m *mockTokenStore) Fetch(ctx context.Context, userURN urn.URN) (*notification.NotificationRequest, error) {
	args := m.Called(ctx, userURN)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*notification.NotificationRequest), args.Error(1)
}

// Reuse mockWebDispatcher from service_integration_test.go if in same package,
// otherwise redefine here. Redefining for safety.
type mockPoisonWebDispatcher struct{}

func (m *mockPoisonWebDispatcher) Dispatch(ctx context.Context, subs []notification.WebPushSubscription, c notification.NotificationContent, d map[string]string) (string, []notification.WebPushSubscription, error) {
	return "", nil, nil
}

// --- Test ---

func TestNotificationService_PoisonPill(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	projectID := "test-project-dlq"

	// 1. Setup Pub/Sub Emulator
	pubsubConn := emulators.SetupPubsubEmulator(t, ctx, emulators.GetDefaultPubsubConfig(projectID))
	psClient, err := pubsub.NewClient(ctx, projectID, pubsubConn.ClientOptions...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = psClient.Close() })

	// 2. Arrange: Resources
	runID := uuid.NewString()
	mainTopicID := "push-main-" + runID
	dlqTopicID := "push-dlq-" + runID
	mainSubID := mainTopicID + "-sub"
	dlqSubID := dlqTopicID + "-sub"

	createPubsubResources(t, ctx, psClient, projectID, dlqTopicID, dlqSubID)
	dlqTopicName := fmt.Sprintf("projects/%s/topics/%s", projectID, dlqTopicID)

	mainTopicName := fmt.Sprintf("projects/%s/topics/%s", projectID, mainTopicID)
	_, err = psClient.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: mainTopicName})
	require.NoError(t, err)

	mainSubName := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, mainSubID)
	mainSub := &pubsubpb.Subscription{
		Name:  mainSubName,
		Topic: mainTopicName,
		DeadLetterPolicy: &pubsubpb.DeadLetterPolicy{
			DeadLetterTopic:     dlqTopicName,
			MaxDeliveryAttempts: 5,
		},
		RetryPolicy: &pubsubpb.RetryPolicy{
			MinimumBackoff: &durationpb.Duration{Seconds: 1},
		},
	}
	_, err = psClient.SubscriptionAdminClient.CreateSubscription(ctx, mainSub)
	require.NoError(t, err)

	// 3. Arrange: Dependencies
	fcmDispatcher := newMockDispatcher(-1) // Helper from integration test file
	webDispatcher := &mockPoisonWebDispatcher{}
	tokenStore := new(mockTokenStore)

	consumerCfg := *messagepipeline.NewGooglePubsubConsumerDefaults(mainSubID)
	consumer, err := messagepipeline.NewGooglePubsubConsumer(&consumerCfg, psClient, logger)
	require.NoError(t, err)

	cfg := &config.Config{
		ProjectID:          projectID,
		ListenAddr:         ":0",
		SubscriptionID:     mainSubID,
		NumPipelineWorkers: 2,
	}

	noopAuth := func(h http.Handler) http.Handler { return h }

	// New Constructor Usage
	notificationService, err := notificationservice.New(
		cfg,
		consumer,
		fcmDispatcher,
		webDispatcher,
		tokenStore,
		noopAuth,
		logger,
	)
	require.NoError(t, err)

	// 4. Act
	serviceCtx, serviceCancel := context.WithCancel(ctx)
	defer serviceCancel()
	go func() {
		_ = notificationService.Start(serviceCtx)
	}()
	t.Cleanup(func() { _ = notificationService.Shutdown(context.Background()) })

	// Publish Poison Pill
	poisonPayload := []byte(`{"this is not valid json"`)
	psClient.Publisher(mainTopicID).Publish(ctx, &pubsub.Message{Data: poisonPayload}).Get(ctx)

	// 5. Assert DLQ
	dlqSub := psClient.Subscriber(dlqSubID)
	var wg sync.WaitGroup
	wg.Add(1)
	var receivedMsg *pubsub.Message

	go func() {
		defer wg.Done()
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		_ = dlqSub.Receive(cctx, func(ctx context.Context, msg *pubsub.Message) {
			msg.Ack()
			receivedMsg = msg
			cancel()
		})
	}()

	wg.Wait()
	require.NotNil(t, receivedMsg, "Did not receive message on the DLQ subscription")
	assert.Equal(t, poisonPayload, receivedMsg.Data)

	// 6. Assert No Dispatch
	assert.Equal(t, 0, fcmDispatcher.GetCallCount())
}
