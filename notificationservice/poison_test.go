// --- File: notificationservice/poison_test.go ---
//go:build integration

package notificationservice_test

import (
	"context"
	"errors"
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
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-notification-service/notificationservice"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"
	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	"github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

// --- Mocks ---

// mockTokenStore is needed to satisfy the New() constructor,
// even though it won't be called in a poison pill scenario (transformer fails first).
type mockTokenStore struct {
	mock.Mock
}

func (m *mockTokenStore) RegisterToken(ctx context.Context, userURN urn.URN, token notification.DeviceToken) error {
	args := m.Called(ctx, userURN, token)
	return args.Error(0)
}

func (m *mockTokenStore) GetTokens(ctx context.Context, userURN urn.URN) ([]notification.DeviceToken, error) {
	args := m.Called(ctx, userURN)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]notification.DeviceToken), args.Error(1)
}

// --- Test ---

func TestNotificationService_PoisonPill(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)

	// Loggers: Slog for the service, Zerolog for the legacy consumer lib
	logger := zerolog.New(zerolog.NewTestWriter(t))
	slogLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	projectID := "test-project-dlq"

	// 1. Setup Pub/Sub Emulator
	pubsubConn := emulators.SetupPubsubEmulator(t, ctx, emulators.GetDefaultPubsubConfig(projectID))
	psClient, err := pubsub.NewClient(ctx, projectID, pubsubConn.ClientOptions...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = psClient.Close() })

	// 2. Arrange: Create main topic, DLQ topic, and subscriptions
	runID := uuid.NewString()
	mainTopicID := "push-main-" + runID
	dlqTopicID := "push-dlq-" + runID
	mainSubID := mainTopicID + "-sub"
	dlqSubID := dlqTopicID + "-sub" // To listen for the dead-lettered message

	// Create the DLQ topic and a subscription for it first
	createPubsubResources(t, ctx, psClient, projectID, dlqTopicID, dlqSubID)
	dlqTopicName := fmt.Sprintf("projects/%s/topics/%s", projectID, dlqTopicID)

	// Create the main topic and subscription WITH the DeadLetterPolicy
	mainTopicName := fmt.Sprintf("projects/%s/topics/%s", projectID, mainTopicID)
	_, err = psClient.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: mainTopicName})
	require.NoError(t, err)

	mainSubName := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, mainSubID)
	mainSub := &pubsubpb.Subscription{
		Name:  mainSubName,
		Topic: mainTopicName,
		DeadLetterPolicy: &pubsubpb.DeadLetterPolicy{
			DeadLetterTopic:     dlqTopicName,
			MaxDeliveryAttempts: 5, // Use a low number for fast test execution
		},
		RetryPolicy: &pubsubpb.RetryPolicy{
			MinimumBackoff: &durationpb.Duration{Seconds: 1}, // Fast retries
		},
	}
	_, err = psClient.SubscriptionAdminClient.CreateSubscription(ctx, mainSub)
	require.NoError(t, err)

	// 3. Arrange: Create service with dependencies
	// Dispatcher (Mock)
	fcmDispatcher := newMockDispatcher(-1)
	dispatchers := map[string]dispatch.Dispatcher{"android": fcmDispatcher}

	// Token Store (Mock - not expected to be called)
	tokenStore := new(mockTokenStore)

	// Consumer (Legacy)
	consumerCfg := *messagepipeline.NewGooglePubsubConsumerDefaults(mainSubID)
	consumer, err := messagepipeline.NewGooglePubsubConsumer(&consumerCfg, psClient, logger)
	require.NoError(t, err)

	// Service Configuration
	cfg := &config.Config{
		ProjectID:          projectID,
		ListenAddr:         ":0",
		SubscriptionID:     mainSubID,
		NumPipelineWorkers: 2,
	}

	// Instantiate Service
	// We pass a no-op auth middleware since we aren't testing the API here
	noopAuth := func(h http.Handler) http.Handler { return h }

	notificationService, err := notificationservice.New(cfg, consumer, dispatchers, tokenStore, noopAuth, slogLogger)
	require.NoError(t, err)

	// 4. Act: Start the service and publish a poison pill message
	serviceCtx, serviceCancel := context.WithCancel(ctx)
	defer serviceCancel()
	go func() {
		if err := notificationService.Start(serviceCtx); err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("service.Start() returned an error: %v", err)
		}
	}()
	t.Cleanup(func() { _ = notificationService.Shutdown(context.Background()) })

	// Publish MALFORMED JSON. This triggers a failure in the Transformer (unmarshal error).
	poisonPayload := []byte(`{"this is not valid json"`)
	result := psClient.Publisher(mainTopicID).Publish(ctx, &pubsub.Message{Data: poisonPayload})
	_, err = result.Get(ctx)
	require.NoError(t, err)
	t.Log("Published poison pill message.")

	// 5. Assert: Verify the message arrives on the DLQ subscription
	dlqSub := psClient.Subscriber(dlqSubID)
	var wg sync.WaitGroup
	wg.Add(1)
	var receivedMsg *pubsub.Message

	go func() {
		defer wg.Done()
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		err = dlqSub.Receive(cctx, func(ctx context.Context, msg *pubsub.Message) {
			msg.Ack()
			receivedMsg = msg
			cancel() // Stop receiving after one message
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("DLQ Receive returned an unexpected error: %v", err)
		}
	}()

	wg.Wait()
	require.NotNil(t, receivedMsg, "Did not receive message on the DLQ subscription")
	assert.Equal(t, poisonPayload, receivedMsg.Data)
	t.Log("✅ Poison pill message correctly received on DLQ.")

	// 6. Negative Assertion: Verify the dispatcher was never called
	assert.Equal(t, 0, fcmDispatcher.GetCallCount(), "Dispatcher should not be called for a poison pill message")
	t.Log("✅ Verified dispatcher was not called.")
}
