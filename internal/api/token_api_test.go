package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tinywideclouds/go-microservice-base/pkg/middleware"

	// Ensure this import path matches your directory structure
	"github.com/tinywideclouds/go-notification-service/internal/api"

	urn "github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// --- Mocks ---
type MockTokenStore struct {
	mock.Mock
}

func (m *MockTokenStore) RegisterFCM(ctx context.Context, u urn.URN, token string) error {
	args := m.Called(ctx, u, token)
	return args.Error(0)
}
func (m *MockTokenStore) RegisterWeb(ctx context.Context, u urn.URN, sub notification.WebPushSubscription) error {
	args := m.Called(ctx, u, sub)
	return args.Error(0)
}
func (m *MockTokenStore) UnregisterFCM(ctx context.Context, u urn.URN, token string) error {
	args := m.Called(ctx, u, token)
	return args.Error(0)
}
func (m *MockTokenStore) UnregisterWeb(ctx context.Context, u urn.URN, endpoint string) error {
	args := m.Called(ctx, u, endpoint)
	return args.Error(0)
}
func (m *MockTokenStore) Fetch(ctx context.Context, u urn.URN) (*notification.NotificationRequest, error) {
	args := m.Called(ctx, u)
	return args.Get(0).(*notification.NotificationRequest), args.Error(1)
}

// --- Setup ---
func setupAPI(t *testing.T) (*api.TokenAPI, *MockTokenStore) {
	mockStore := new(MockTokenStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return api.NewTokenAPI(mockStore, logger), mockStore
}

// Helper to inject UserID into context (simulating Auth Middleware)
func withUser(req *http.Request, userID string) *http.Request {
	// FIX: Use the exported helper from middleware package
	// Trying to use 'middleware.UserIDKey' directly is illegal as the key is private.
	ctx := middleware.ContextWithUserID(req.Context(), userID)
	return req.WithContext(ctx)
}

// --- Tests ---

func TestRegisterFCM(t *testing.T) {
	apiHandler, mockStore := setupAPI(t)
	targetURN, _ := urn.Parse("urn:test:user:123")

	t.Run("Success", func(t *testing.T) {
		payload := map[string]string{"token": "fcm-token-abc"}
		body, _ := json.Marshal(payload)

		// Use fixed helper
		req := withUser(httptest.NewRequest("POST", "/register/fcm", bytes.NewReader(body)), targetURN.String())
		w := httptest.NewRecorder()

		// Expectation: Store receives the string directly
		mockStore.On("RegisterFCM", mock.Anything, targetURN, "fcm-token-abc").Return(nil)

		apiHandler.RegisterFCM(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		mockStore.AssertExpectations(t)
	})

	t.Run("Rejects Empty Token", func(t *testing.T) {
		payload := map[string]string{"token": ""} // Empty
		body, _ := json.Marshal(payload)
		req := withUser(httptest.NewRequest("POST", "/register/fcm", bytes.NewReader(body)), targetURN.String())
		w := httptest.NewRecorder()

		apiHandler.RegisterFCM(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestRegisterWeb(t *testing.T) {
	apiHandler, mockStore := setupAPI(t)
	targetURN, _ := urn.Parse("urn:test:user:123")

	validSub := notification.WebPushSubscription{
		Endpoint: "https://fcm.googleapis.com/fcm/send/xyz",
		Keys: struct {
			P256dh []byte `json:"p256dh"`
			Auth   []byte `json:"auth"`
		}{
			P256dh: []byte{0xDE, 0xAD, 0xBE, 0xEF},
			Auth:   []byte{0xCA, 0xFE, 0xBA, 0xBE},
		},
	}

	t.Run("Success", func(t *testing.T) {
		body, _ := json.Marshal(validSub)
		req := withUser(httptest.NewRequest("POST", "/register/web", bytes.NewReader(body)), targetURN.String())
		w := httptest.NewRecorder()

		// Expectation: Store receives the full struct
		mockStore.On("RegisterWeb", mock.Anything, targetURN, validSub).Return(nil)

		apiHandler.RegisterWeb(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		mockStore.AssertExpectations(t)
	})

	t.Run("Rejects Missing Keys (Invalid Object)", func(t *testing.T) {
		// Missing 'keys'
		invalidPayload := `{"endpoint": "https://valid.com"}`
		req := withUser(httptest.NewRequest("POST", "/register/web", bytes.NewReader([]byte(invalidPayload))), targetURN.String())
		w := httptest.NewRecorder()

		apiHandler.RegisterWeb(w, req)

		// Should detect incomplete object
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
