// --- File: internal/platform/apns/dispatcher_internal_test.go ---
package apns

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/sideshow/apns2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// MockAPNSClient definition repeated here for internal test visibility
type MockAPNSClient struct {
	mock.Mock
}

func (m *MockAPNSClient) Push(n *apns2.Notification) (*apns2.Response, error) {
	args := m.Called(n)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*apns2.Response), args.Error(1)
}

func TestDispatch_Internal(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	content := notification.NotificationContent{Title: "Hello iOS"}
	data := map[string]string{"msg_id": "123"}

	t.Run("Happy Path - Success", func(t *testing.T) {
		mockClient := new(MockAPNSClient)
		dispatcher := &Dispatcher{
			client: mockClient,
			topic:  "com.test.app",
			logger: logger,
		}

		tokens := []string{"token-1"}

		// Arrange: Return 200 OK
		mockResponse := &apns2.Response{StatusCode: http.StatusOK}
		mockClient.On("Push", mock.MatchedBy(func(n *apns2.Notification) bool {
			return n.DeviceToken == "token-1" && n.Topic == "com.test.app"
		})).Return(mockResponse, nil)

		// Act
		receipt, invalid, err := dispatcher.Dispatch(ctx, tokens, content, data)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, invalid)
		assert.Contains(t, receipt, "success:1")
		mockClient.AssertExpectations(t)
	})

	t.Run("Self-Healing - Bad Device Token", func(t *testing.T) {
		mockClient := new(MockAPNSClient)
		dispatcher := &Dispatcher{
			client: mockClient,
			topic:  "com.test.app",
			logger: logger,
		}

		tokens := []string{"bad-token"}

		// Arrange: Return 400 BadDeviceToken
		mockResponse := &apns2.Response{
			StatusCode: http.StatusBadRequest,
			Reason:     apns2.ReasonBadDeviceToken,
		}
		mockClient.On("Push", mock.Anything).Return(mockResponse, nil)

		// Act
		_, invalid, err := dispatcher.Dispatch(ctx, tokens, content, data)

		// Assert
		require.NoError(t, err)
		assert.Len(t, invalid, 1)
		assert.Equal(t, "bad-token", invalid[0])
	})

	t.Run("Transport Failure - Retryable", func(t *testing.T) {
		mockClient := new(MockAPNSClient)
		dispatcher := &Dispatcher{
			client: mockClient,
			topic:  "com.test.app",
			logger: logger,
		}

		tokens := []string{"token-1"}

		// Arrange: Return Error (Network down)
		mockClient.On("Push", mock.Anything).Return(nil, errors.New("connection refused"))

		// Act
		receipt, invalid, err := dispatcher.Dispatch(ctx, tokens, content, data)

		// Assert
		// Note: The current implementation logs transport errors and continues, returning nil error.
		// This is a design choice (best effort).
		require.NoError(t, err)
		assert.Empty(t, invalid)
		assert.Contains(t, receipt, "total_fail:1")
	})
}
