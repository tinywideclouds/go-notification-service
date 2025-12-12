// --- File: internal/pipeline/processor_test.go ---
package pipeline_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-notification-service/internal/pipeline"
	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	"github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- Mocks ---
type mockDispatcher struct {
	dispatchFunc func(ctx context.Context, tokens []string, content notification.NotificationContent, data map[string]string) error
	callCount    int
	lastTokens   []string
}

func (m *mockDispatcher) Dispatch(ctx context.Context, tokens []string, content notification.NotificationContent, data map[string]string) error {
	m.callCount++
	m.lastTokens = tokens
	if m.dispatchFunc != nil {
		return m.dispatchFunc(ctx, tokens, content, data)
	}
	return nil
}

type mockTokenStore struct {
	mock.Mock
}

func (m *mockTokenStore) RegisterToken(ctx context.Context, userURN urn.URN, token notification.DeviceToken) error {
	return m.Called(ctx, userURN, token).Error(0)
}

func (m *mockTokenStore) GetTokens(ctx context.Context, userURN urn.URN) ([]notification.DeviceToken, error) {
	args := m.Called(ctx, userURN)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]notification.DeviceToken), args.Error(1)
}

// --- Tests ---

func TestProcessor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	logger := newTestLogger()
	testURN, _ := urn.Parse("urn:sm:user:test-user")

	t.Run("routes to correct dispatchers after lookup", func(t *testing.T) {
		// Arrange
		fcmDispatcher := &mockDispatcher{}
		apnsDispatcher := &mockDispatcher{}
		dispatchers := map[string]dispatch.Dispatcher{
			"android": fcmDispatcher,
			"ios":     apnsDispatcher,
		}

		tokenStore := new(mockTokenStore)
		// Mock the lookup: The processor asks for tokens, we return them.
		foundTokens := []notification.DeviceToken{
			{Token: "android-1", Platform: "android"},
			{Token: "ios-1", Platform: "ios"},
		}
		tokenStore.On("GetTokens", mock.Anything, testURN).Return(foundTokens, nil)

		processor := pipeline.NewProcessor(dispatchers, tokenStore, logger)

		// Request comes in WITHOUT tokens now
		request := &notification.NotificationRequest{
			RecipientID: testURN,
			// Tokens: []... (Empty)
		}

		// Act
		err := processor(ctx, messagepipeline.Message{}, request)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, 1, fcmDispatcher.callCount)
		assert.Equal(t, []string{"android-1"}, fcmDispatcher.lastTokens)
		assert.Equal(t, 1, apnsDispatcher.callCount)
		assert.Equal(t, []string{"ios-1"}, apnsDispatcher.lastTokens)
	})

	t.Run("drops notification if no devices found", func(t *testing.T) {
		dispatchers := map[string]dispatch.Dispatcher{}
		tokenStore := new(mockTokenStore)
		tokenStore.On("GetTokens", mock.Anything, testURN).Return([]notification.DeviceToken{}, nil)

		processor := pipeline.NewProcessor(dispatchers, tokenStore, logger)
		request := &notification.NotificationRequest{RecipientID: testURN}

		err := processor(ctx, messagepipeline.Message{}, request)
		require.NoError(t, err) // No error, just dropped
	})
}
