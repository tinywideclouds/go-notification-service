// Package service contains the logic for assembling the core application pipeline.
package notificationservice

import (
	"context"
	"fmt"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/illmade-knight/go-microservice-base/pkg/microservice"
	"github.com/illmade-knight/go-notification-service/internal/pipeline"
	"github.com/illmade-knight/go-notification-service/pkg/notification"
	"github.com/illmade-knight/go-secure-messaging/pkg/transport"
	"github.com/rs/zerolog"
)

// Wrapper now embeds BaseServer to get standard server functionality.
type Wrapper struct {
	*microservice.BaseServer
	pipelineService *messagepipeline.StreamingService[transport.NotificationRequest]
	logger          zerolog.Logger
}

// New assembles and wires up the entire notification service.
func New(
	listenAddr string,
	numWorkers int,
	consumer messagepipeline.MessageConsumer,
	dispatchers map[string]notification.Dispatcher,
	logger zerolog.Logger,
) (*Wrapper, error) {
	// 1. Create the standard base server.
	baseServer := microservice.NewBaseServer(logger, listenAddr)

	// 2. Create the message handler (the processor).
	processor := pipeline.NewProcessor(dispatchers, logger)

	// 3. Assemble the core processing pipeline.
	streamingService, err := messagepipeline.NewStreamingService[transport.NotificationRequest](
		messagepipeline.StreamingServiceConfig{NumWorkers: numWorkers},
		consumer,
		pipeline.NotificationRequestTransformer,
		processor,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create streaming service: %w", err)
	}

	return &Wrapper{
		BaseServer:      baseServer,
		pipelineService: streamingService,
		logger:          logger,
	}, nil
}

// Start runs the service's background pipeline before starting the base HTTP server.
func (w *Wrapper) Start(ctx context.Context) error {
	w.logger.Info().Msg("Core processing pipeline starting...")
	if err := w.pipelineService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start processing service: %w", err)
	}

	// Once the pipeline is running, the service is ready to be exposed.
	w.SetReady(true)
	w.logger.Info().Msg("Service is now ready.")

	return w.BaseServer.Start()
}

// Shutdown gracefully stops all service components in the correct order.
func (w *Wrapper) Shutdown(ctx context.Context) error {
	w.logger.Info().Msg("Shutting down service components...")
	var finalErr error

	if err := w.pipelineService.Stop(ctx); err != nil {
		w.logger.Error().Err(err).Msg("Processing pipeline shutdown failed.")
		finalErr = err
	}

	if err := w.BaseServer.Shutdown(ctx); err != nil {
		w.logger.Error().Err(err).Msg("HTTP server shutdown failed.")
		finalErr = err
	}

	w.logger.Info().Msg("Service shutdown complete.")
	return finalErr
}
