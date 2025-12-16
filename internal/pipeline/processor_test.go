package pipeline_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-notification-service/internal/pipeline"
	urn "github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- Typed Mocks ---

// Mock for FCM (String-based)
type mockFCMDispatcher struct {
	mock.Mock
}

func (m *mockFCMDispatcher) Dispatch(ctx context.Context, tokens []string, content notification.NotificationContent, data map[string]string) (string, []string, error) {
	args := m.Called(ctx, tokens, content, data)
	return args.String(0), args.Get(1).([]string), args.Error(2)
}

// Mock for Web (Object-based)
type mockWebDispatcher struct {
	mock.Mock
}

func (m *mockWebDispatcher) Dispatch(ctx context.Context, subs []notification.WebPushSubscription, content notification.NotificationContent, data map[string]string) (string, []notification.WebPushSubscription, error) {
	args := m.Called(ctx, subs, content, data)
	return args.String(0), args.Get(1).([]notification.WebPushSubscription), args.Error(2)
}

type mockTokenStore struct {
	mock.Mock
}

// Implement only what Processor uses
func (m *mockTokenStore) Fetch(ctx context.Context, user urn.URN) (*notification.NotificationRequest, error) {
	args := m.Called(ctx, user)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*notification.NotificationRequest), args.Error(1)
}
func (m *mockTokenStore) UnregisterFCM(ctx context.Context, user urn.URN, token string) error {
	return m.Called(ctx, user, token).Error(0)
}
func (m *mockTokenStore) UnregisterWeb(ctx context.Context, user urn.URN, endpoint string) error {
	return m.Called(ctx, user, endpoint).Error(0)
}

// Satisfy strict interface (stubs for unused methods)
func (m *mockTokenStore) RegisterFCM(_ context.Context, _ urn.URN, _ string) error { return nil }
func (m *mockTokenStore) RegisterWeb(_ context.Context, _ urn.URN, _ notification.WebPushSubscription) error {
	return nil
}

func TestProcessor_Routing(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()
	testURN, _ := urn.Parse("urn:sm:user:test-processor")

	// Input Message (Content only, no tokens)
	inboundReq := &notification.NotificationRequest{
		RecipientID: testURN,
		Content:     notification.NotificationContent{Title: "Hello"},
	}

	t.Run("Routes Mixed Traffic Correctly", func(t *testing.T) {
		fcmMock := new(mockFCMDispatcher)
		webMock := new(mockWebDispatcher)
		storeMock := new(mockTokenStore)

		// 1. Setup Store Response (The Fan-Out)
		// Return 1 FCM token and 1 Web Subscription
		populatedReq := &notification.NotificationRequest{
			RecipientID: testURN,
			FCMTokens:   []string{"fcm-123"},
			WebSubscriptions: []notification.WebPushSubscription{
				{Endpoint: "https://web.push/abc"},
			},
		}
		storeMock.On("Fetch", mock.Anything, testURN).Return(populatedReq, nil)

		// 2. Setup Dispatch Expectations
		fcmMock.On("Dispatch", mock.Anything, []string{"fcm-123"}, inboundReq.Content, mock.Anything).
			Return("ok", []string{}, nil)

		webMock.On("Dispatch", mock.Anything, populatedReq.WebSubscriptions, inboundReq.Content, mock.Anything).
			Return("ok", []notification.WebPushSubscription{}, nil)

		// 3. Execute
		processor := pipeline.NewProcessor(fcmMock, webMock, storeMock, logger)
		err := processor(ctx, messagepipeline.Message{}, inboundReq)

		// 4. Verify
		require.NoError(t, err)
		fcmMock.AssertExpectations(t)
		webMock.AssertExpectations(t)
	})

	t.Run("Self-Healing Web Cleanup", func(t *testing.T) {
		fcmMock := new(mockFCMDispatcher) // Not used
		webMock := new(mockWebDispatcher)
		storeMock := new(mockTokenStore)

		// 1. Store returns 1 Web Sub
		badSub := notification.WebPushSubscription{Endpoint: "https://dead.endpoint"}
		populatedReq := &notification.NotificationRequest{
			WebSubscriptions: []notification.WebPushSubscription{badSub},
		}
		storeMock.On("Fetch", mock.Anything, testURN).Return(populatedReq, nil)

		// 2. Dispatcher reports it as INVALID (410/404)
		webMock.On("Dispatch", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return("failed", []notification.WebPushSubscription{badSub}, nil)

		// 3. Processor MUST call UnregisterWeb
		storeMock.On("UnregisterWeb", mock.Anything, testURN, "https://dead.endpoint").Return(nil)

		// Execute
		processor := pipeline.NewProcessor(fcmMock, webMock, storeMock, logger)
		err := processor(ctx, messagepipeline.Message{}, inboundReq)

		require.NoError(t, err)
		storeMock.AssertExpectations(t)
	})
}
