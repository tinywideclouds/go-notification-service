// --- File: internal/api/token_api.go ---
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/tinywideclouds/go-microservice-base/pkg/middleware"
	"github.com/tinywideclouds/go-microservice-base/pkg/response"
	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	"github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

// TokenAPI handles device registration requests.
type TokenAPI struct {
	Store  dispatch.TokenStore
	Logger *slog.Logger
}

func NewTokenAPI(store dispatch.TokenStore, logger *slog.Logger) *TokenAPI {
	return &TokenAPI{Store: store, Logger: logger}
}

// RegisterTokenHandler handles PUT /tokens
// It extracts the user from the JWT and saves the token from the body.
func (a *TokenAPI) RegisterTokenHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Auth check
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok || userID == "" {
		response.WriteJSONError(w, http.StatusUnauthorized, "missing user identity")
		return
	}
	userURN, err := urn.Parse(userID)
	if err != nil {
		a.Logger.Warn("Invalid user URN in token", "user_id", userID)
		response.WriteJSONError(w, http.StatusUnauthorized, "invalid user identity format")
		return
	}

	// 2. Decode body
	var tokenReq notification.DeviceToken
	if err := json.NewDecoder(r.Body).Decode(&tokenReq); err != nil {
		response.WriteJSONError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if tokenReq.Token == "" || tokenReq.Platform == "" {
		response.WriteJSONError(w, http.StatusBadRequest, "token and platform are required")
		return
	}

	// 3. Register
	if err := a.Store.RegisterToken(r.Context(), userURN, tokenReq); err != nil {
		a.Logger.Error("Failed to register token", "err", err)
		response.WriteJSONError(w, http.StatusInternalServerError, "failed to register token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
