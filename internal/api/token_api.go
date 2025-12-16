package api

import (
	"encoding/json"
	"net/http"

	"log/slog"

	"github.com/tinywideclouds/go-microservice-base/pkg/middleware"
	"github.com/tinywideclouds/go-microservice-base/pkg/response"
	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	urn "github.com/tinywideclouds/go-platform/pkg/net/v1"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

type TokenAPI struct {
	Store  dispatch.TokenStore
	Logger *slog.Logger
}

func NewTokenAPI(store dispatch.TokenStore, logger *slog.Logger) *TokenAPI {
	return &TokenAPI{
		Store:  store,
		Logger: logger,
	}
}

// --- DOOR A: Mobile (FCM) ---

type RegisterFCMRequest struct {
	Token string `json:"token"`
}

func (api *TokenAPI) RegisterFCM(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := middleware.GetUserHandleFromContext(ctx)
	if !ok {
		response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userURN, _ := urn.Parse(userID)

	var req RegisterFCMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if req.Token == "" {
		response.WriteJSONError(w, http.StatusBadRequest, "missing token")
		return
	}

	// Direct call to FCM storage logic
	if err := api.Store.RegisterFCM(ctx, userURN, req.Token); err != nil {
		api.Logger.Error("failed to register fcm", "err", err)
		response.WriteJSONError(w, http.StatusInternalServerError, "storage failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- DOOR B: Web (VAPID) ---

func (api *TokenAPI) RegisterWeb(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := middleware.GetUserHandleFromContext(ctx)
	if !ok {
		response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userURN, _ := urn.Parse(userID)

	// We decode directly into the Domain Object we defined in the previous step
	var sub notification.WebPushSubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		api.Logger.Error("RegisterWeb: JSON Decode failed", "err", err)
		response.WriteJSONError(w, http.StatusBadRequest, "invalid subscription json")
		return
	}

	// Validate the Web Object (The "Big JSON" keys must exist)
	if sub.Endpoint == "" || len(sub.Keys.P256dh) == 0 || len(sub.Keys.Auth) == 0 {
		api.Logger.Warn("RegisterWeb: Validation failed", "reason", "missing fields")
		response.WriteJSONError(w, http.StatusBadRequest, "incomplete subscription object")
		return
	}

	// Direct call to Web storage logic
	if err := api.Store.RegisterWeb(ctx, userURN, sub); err != nil {
		api.Logger.Error("failed to register web", "err", err)
		response.WriteJSONError(w, http.StatusInternalServerError, "storage failed")
		return
	}
	api.Logger.Info("RegisterWeb: Subscription registered", "user", userURN, "endpoint", sub.Endpoint)

	// Success

	w.WriteHeader(http.StatusNoContent)
}

func (api *TokenAPI) UnregisterFCM(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := middleware.GetUserHandleFromContext(ctx)
	if !ok {
		response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userURN, _ := urn.Parse(userID)

	var req RegisterFCMRequest // We can reuse the struct since it just holds "token"
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := api.Store.UnregisterFCM(ctx, userURN, req.Token); err != nil {
		// Log but don't fail hard; idempotency is preferred for unregister
		api.Logger.Warn("failed to unregister fcm", "err", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- UNREGISTER DOOR B: Web (VAPID) ---

type UnregisterWebRequest struct {
	Endpoint string `json:"endpoint"`
}

func (api *TokenAPI) UnregisterWeb(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := middleware.GetUserHandleFromContext(ctx)
	if !ok {
		response.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userURN, _ := urn.Parse(userID)

	var req UnregisterWebRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Logger.Error("UnregisterWeb: JSON Decode failed", "err", err)
		response.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}

	// We only need the Endpoint URL to identify and delete the row
	if req.Endpoint == "" {
		api.Logger.Warn("UnregisterWeb: Validation failed", "reason", "missing endpoint")
		response.WriteJSONError(w, http.StatusBadRequest, "missing endpoint")
		return
	}

	if err := api.Store.UnregisterWeb(ctx, userURN, req.Endpoint); err != nil {
		api.Logger.Warn("failed to unregister web", "err", err)
		response.WriteJSONError(w, http.StatusInternalServerError, "failed to unregister web")
		return
	}
	api.Logger.Info("UnregisterWeb: Subscription unregistered", "user", userURN, "endpoint", req.Endpoint)

	// Success

	w.WriteHeader(http.StatusNoContent)
}
