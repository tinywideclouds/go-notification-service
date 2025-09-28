package pipeline_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/illmade-knight/go-notification-service/internal/pipeline"
	"github.com/illmade-knight/go-notification-service/pkg/notification"
	"github.com/illmade-knight/go-secure-messaging/pkg/transport"
	"github.com/illmade-knight/go-secure-messaging/pkg/urn"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDispatcher is a test double for the notification.Dispatcher interface.
type mockDispatcher struct {
	dispatchFunc func(ctx context.Context, tokens []string, content transport.NotificationContent, data map[string]string) error
	callCount    int
	lastTokens   []string
}

func (m *mockDispatcher) Dispatch(ctx context.Context, tokens []string, content transport.NotificationContent, data map[string]string) error {
	m.callCount++
	m.lastTokens = tokens
	if m.dispatchFunc != nil {
		return m.dispatchFunc(ctx, tokens, content, data)
	}
	return nil
}

func TestProcessor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	testURN, err := urn.Parse("urn:sm:user:test-user")
	require.NoError(t, err)

	t.Run("routes to correct dispatchers by platform", func(t *testing.T) {
		fcmDispatcher := &mockDispatcher{}
		apnsDispatcher := &mockDispatcher{}
		dispatchers := map[string]notification.Dispatcher{
			"android": fcmDispatcher,
			"ios":     apnsDispatcher,
		}

		processor := pipeline.NewProcessor(dispatchers, zerolog.Nop())

		request := &transport.NotificationRequest{
			RecipientID: testURN,
			Tokens: []transport.DeviceToken{
				{Token: "android-1", Platform: "android"},
				{Token: "ios-1", Platform: "ios"},
				{Token: "android-2", Platform: "android"},
				{Token: "web-1", Platform: "web"}, // This platform has no dispatcher
			},
		}

		err := processor(ctx, messagepipeline.Message{}, request)
		require.NoError(t, err)

		assert.Equal(t, 1, fcmDispatcher.callCount, "FCM dispatcher should be called once")
		assert.ElementsMatch(t, []string{"android-1", "android-2"}, fcmDispatcher.lastTokens)

		assert.Equal(t, 1, apnsDispatcher.callCount, "APNS dispatcher should be called once")
		assert.ElementsMatch(t, []string{"ios-1"}, apnsDispatcher.lastTokens)
	})

	t.Run("returns error if a dispatcher fails", func(t *testing.T) {
		expectedErr := errors.New("fcm failed")
		fcmDispatcher := &mockDispatcher{
			dispatchFunc: func(ctx context.Context, tokens []string, content transport.NotificationContent, data map[string]string) error {
				return expectedErr
			},
		}
		dispatchers := map[string]notification.Dispatcher{"android": fcmDispatcher}
		processor := pipeline.NewProcessor(dispatchers, zerolog.Nop())

		request := &transport.NotificationRequest{
			RecipientID: testURN,
			Tokens:      []transport.DeviceToken{{Token: "android-1", Platform: "android"}},
		}

		err := processor(ctx, messagepipeline.Message{}, request)
		require.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}
