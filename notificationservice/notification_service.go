// --- File: notificationservice/service.go ---
package notificationservice

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/tinywideclouds/go-microservice-base/pkg/microservice"
	"github.com/tinywideclouds/go-microservice-base/pkg/middleware"
	"github.com/tinywideclouds/go-notification-service/internal/api"
	"github.com/tinywideclouds/go-notification-service/internal/pipeline"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"
	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	notification "github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

type Wrapper struct {
	*microservice.BaseServer
	pipelineService *messagepipeline.StreamingService[notification.NotificationRequest]
	logger          *slog.Logger
}

// New assembles the service.
func New(
	cfg *config.Config,
	consumer messagepipeline.MessageConsumer,
	fcmDispatcher dispatch.Dispatcher,
	webDispatcher dispatch.WebDispatcher,
	tokenStore dispatch.TokenStore,
	authMiddleware func(http.Handler) http.Handler,
	logger *slog.Logger,
) (*Wrapper, error) {

	// 1. Base Server
	baseServer := microservice.NewBaseServer(logger, cfg.ListenAddr)

	// 2. Processor
	processor := pipeline.NewProcessor(fcmDispatcher, webDispatcher, tokenStore, logger)

	// 3. Pipeline
	streamingService, err := messagepipeline.NewStreamingService(
		messagepipeline.StreamingServiceConfig{NumWorkers: cfg.NumPipelineWorkers},
		consumer,
		pipeline.NotificationRequestTransformer,
		processor,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create streaming service: %w", err)
	}

	// 4. API (Token Registration)
	tokenAPI := api.NewTokenAPI(tokenStore, logger)

	// Register Routes
	mux := baseServer.Mux()
	corsMiddleware := middleware.NewCorsMiddleware(cfg.CorsConfig, logger)

	// --- REFACTORED ROUTES START ---

	// Helper for clean route definition
	handle := func(pattern string, handlerFunc http.HandlerFunc) {
		mux.Handle(pattern, corsMiddleware(authMiddleware(handlerFunc)))
	}

	// 1. Registration Paths (Segregated)
	handle("POST /api/v1/register/fcm", tokenAPI.RegisterFCM)
	handle("POST /api/v1/register/web", tokenAPI.RegisterWeb)

	// 2. Unregistration Paths (Segregated)
	handle("POST /api/v1/unregister/fcm", tokenAPI.UnregisterFCM)
	handle("POST /api/v1/unregister/web", tokenAPI.UnregisterWeb)

	// 3. Global OPTIONS for the API namespace (CORS preflight)
	mux.Handle("OPTIONS /api/v1/", corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just returns 200 OK with CORS headers handled by middleware
	})))

	// --- REFACTORED ROUTES END ---

	return &Wrapper{
		BaseServer:      baseServer,
		pipelineService: streamingService,
		logger:          logger,
	}, nil
}

// Start and Shutdown remain unchanged
func (w *Wrapper) Start(ctx context.Context) error {
	w.logger.Info("Core processing pipeline starting...")
	if err := w.pipelineService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start processing service: %w", err)
	}
	w.SetReady(true)
	w.logger.Info("Service is now ready.")
	return w.BaseServer.Start()
}

func (w *Wrapper) Shutdown(ctx context.Context) error {
	w.logger.Info("Shutting down service components...")
	var finalErr error
	if err := w.pipelineService.Stop(ctx); err != nil {
		w.logger.Error("Processing pipeline shutdown failed.", "err", err)
		finalErr = err
	}
	if err := w.BaseServer.Shutdown(ctx); err != nil {
		w.logger.Error("HTTP server shutdown failed.", "err", err)
		finalErr = err
	}
	w.logger.Info("Service shutdown complete.")
	return finalErr
}
