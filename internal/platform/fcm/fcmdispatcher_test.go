// --- File: internal/platform/fcm/dispatcher_test.go ---
package fcm_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"firebase.google.com/go/v4/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-notification-service/internal/platform/fcm"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// MockClient satisfies the new MessagingClient interface
type MockClient struct {
	mock.Mock
}

func (m *MockClient) SendEachForMulticast(ctx context.Context, msg *messaging.MulticastMessage) (*messaging.BatchResponse, error) {
	args := m.Called(ctx, msg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*messaging.BatchResponse), args.Error(1)
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestFCMDispatch_Lifecycle(t *testing.T) {
	logger := newTestLogger()
	ctx := context.Background()
	content := notification.NotificationContent{Title: "Test"}
	data := map[string]string{"id": "1"}

	t.Run("Happy Path - All Success", func(t *testing.T) {
		mockClient := new(MockClient)
		dispatcher := fcm.NewDispatcher(mockClient, logger)
		tokens := []string{"token-1", "token-2"}

		// Arrange: Return success for both
		mockResponse := &messaging.BatchResponse{
			SuccessCount: 2,
			FailureCount: 0,
			Responses: []*messaging.SendResponse{
				{Success: true, MessageID: "msg-1"},
				{Success: true, MessageID: "msg-2"},
			},
		}
		mockClient.On("SendEachForMulticast", ctx, mock.Anything).Return(mockResponse, nil)

		// Act
		receipt, invalid, err := dispatcher.Dispatch(ctx, tokens, content, data)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, invalid)
		assert.Contains(t, receipt, "success:2")
		mockClient.AssertExpectations(t)
	})

	t.Run("Transport Failure (Retryable)", func(t *testing.T) {
		mockClient := new(MockClient)
		dispatcher := fcm.NewDispatcher(mockClient, logger)
		tokens := []string{"token-1"}

		// Arrange: Whole batch fails (e.g. DNS error)
		mockClient.On("SendEachForMulticast", ctx, mock.Anything).Return(nil, errors.New("network down"))

		// Act
		_, _, err := dispatcher.Dispatch(ctx, tokens, content, data)

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transport failed")
	})

	// Note: We rely on the Integration Test to verify the specific parsing of
	// IsRegistrationTokenNotRegistered errors, as mocking the internal error types
	// of the Firebase SDK is brittle.
}
