// --- File: notificationservice/service.go ---
package notificationservice

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/tinywideclouds/go-microservice-base/pkg/microservice"
	"github.com/tinywideclouds/go-microservice-base/pkg/middleware"
	"github.com/tinywideclouds/go-notification-service/internal/api"
	"github.com/tinywideclouds/go-notification-service/internal/pipeline"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"
	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	"github.com/tinywideclouds/go-platform/pkg/notification/v1"
)

type Wrapper struct {
	*microservice.BaseServer
	pipelineService *messagepipeline.StreamingService[notification.NotificationRequest]
	logger          *slog.Logger
}

// New assembles the service.
// REFACTORED: Accepts TokenStore and Auth Middleware.
func New(
	cfg *config.Config,
	consumer messagepipeline.MessageConsumer,
	dispatchers map[string]dispatch.Dispatcher,
	tokenStore dispatch.TokenStore, // <-- ADDED
	authMiddleware func(http.Handler) http.Handler, // <-- ADDED
	logger *slog.Logger,
) (*Wrapper, error) {

	// 1. Base Server
	baseServer := microservice.NewBaseServer(logger, cfg.ListenAddr)

	// 2. Processor (Now gets the TokenStore)
	processor := pipeline.NewProcessor(dispatchers, tokenStore, logger)

	// 3. Pipeline
	streamingService, err := messagepipeline.NewStreamingService[notification.NotificationRequest](
		messagepipeline.StreamingServiceConfig{NumWorkers: cfg.NumPipelineWorkers},
		consumer,
		pipeline.NotificationRequestTransformer,
		processor,
		zerolog.Nop(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create streaming service: %w", err)
	}

	// 4. API (Token Registration)
	tokenAPI := api.NewTokenAPI(tokenStore, logger)

	// Register Routes
	mux := baseServer.Mux()

	corsMiddleware := middleware.NewCorsMiddleware(cfg.CorsConfig, logger)

	// OPTIONS
	mux.Handle("OPTIONS /tokens", corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))

	// PUT /tokens (Protected)
	registerHandler := http.HandlerFunc(tokenAPI.RegisterTokenHandler)
	mux.Handle("PUT /tokens", corsMiddleware(authMiddleware(registerHandler)))

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
