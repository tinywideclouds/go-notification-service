// --- File: internal/api/token_api_test.go ---
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tinywideclouds/go-microservice-base/pkg/middleware"
	"github.com/tinywideclouds/go-notification-service/internal/api"
	"github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// Mock TokenStore (reused or redefined)
type mockTokenStore struct {
	mock.Mock
}

func (m *mockTokenStore) RegisterToken(ctx context.Context, userURN urn.URN, token notification.DeviceToken) error {
	args := m.Called(ctx, userURN, token)
	return args.Error(0)
}
func (m *mockTokenStore) GetTokens(ctx context.Context, userURN urn.URN) ([]notification.DeviceToken, error) {
	return nil, nil // Not used in this test
}

func TestRegisterTokenHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := new(mockTokenStore)
	handler := api.NewTokenAPI(store, logger)

	userID := "urn:sm:user:test-123"
	userURN, _ := urn.Parse(userID)

	t.Run("Success", func(t *testing.T) {
		body := notification.DeviceToken{
			Token:    "fcm-token-123",
			Platform: "web",
		}
		jsonBody, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPut, "/tokens", bytes.NewBuffer(jsonBody))
		// Inject Auth Context
		ctx := middleware.ContextWithUserID(req.Context(), userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		// Expectation
		store.On("RegisterToken", mock.Anything, userURN, body).Return(nil).Once()

		handler.RegisterTokenHandler(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		store.AssertExpectations(t)
	})

	t.Run("Unauthorized - Missing Context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/tokens", nil)
		w := httptest.NewRecorder()

		handler.RegisterTokenHandler(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
