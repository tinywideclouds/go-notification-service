package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "embed" // Required for go:embed

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/illmade-knight/go-notification-service/cmd"
	"github.com/illmade-knight/go-notification-service/internal/platform/apns"
	"github.com/illmade-knight/go-notification-service/internal/platform/fcm"
	"github.com/illmade-knight/go-notification-service/notificationservice"
	"github.com/illmade-knight/go-notification-service/pkg/notification"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"gopkg.in/yaml.v3"
)

//go:embed config.yaml
var configYAML []byte

// loadConfig loads and validates the application configuration.
func loadConfig(logger zerolog.Logger) *cmd.AppConfig {
	var cfg cmd.YamlConfig
	err := yaml.Unmarshal(configYAML, &cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to parse embedded config.yaml")
	}

	consumerCfg := messagepipeline.NewGooglePubsubConsumerDefaults(cfg.SubscriptionID)

	finalConfig := &cmd.AppConfig{
		ProjectID:              cfg.ProjectID,
		ListenAddr:             cfg.ListenAddr,
		SubscriptionID:         cfg.SubscriptionID,
		SubscriptionDLQTopicID: cfg.SubscriptionDLQTopicID,
		NumPipelineWorkers:     cfg.NumPipelineWorkers,
		PubsubConsumerConfig:   consumerCfg,
	}

	if finalConfig.ProjectID == "" {
		logger.Fatal().Msg("FATAL: project_id is not set in config.yaml.")
	}
	if finalConfig.SubscriptionID == "" {
		logger.Fatal().Msg("FATAL: subscription_id is not set in config.yaml.")
	}
	if finalConfig.SubscriptionDLQTopicID == "" {
		logger.Warn().Msg("WARN: subscription_dlq_topic_id is not set. Poison pill messages will be retried indefinitely.")
	}

	logger.Info().Str("project_id", finalConfig.ProjectID).Msg("Configuration loaded.")
	return finalConfig
}

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := loadConfig(logger)

	psClient, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create Pub/Sub client")
	}
	defer func() {
		_ = psClient.Close()
	}()

	err = setupPubsubSubscription(ctx, psClient, cfg, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to set up Pub/Sub resources")
	}

	fcmDispatcher, _ := fcm.NewLoggingDispatcher(logger)
	apnsDispatcher, _ := apns.NewLoggingDispatcher(logger)
	dispatchers := map[string]notification.Dispatcher{
		"android": fcmDispatcher,
		"ios":     apnsDispatcher,
	}

	consumer, err := messagepipeline.NewGooglePubsubConsumer(cfg.PubsubConsumerConfig, psClient, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create pubsub consumer")
	}

	notificationService, err := notificationservice.New(
		cfg.ListenAddr,
		cfg.NumPipelineWorkers,
		consumer,
		dispatchers,
		logger,
	)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create notification service")
	}

	errs := make(chan error, 1)
	go func() {
		errs <- notificationService.Start(ctx)
	}()

	select {
	case err := <-errs:
		logger.Error().Err(err).Msg("Notification service failed to start")
		return
	case <-time.After(2 * time.Second):
		logger.Info().Msg("Notification service is running.")
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	<-shutdown
	logger.Info().Msg("Received shutdown signal.")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := notificationService.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("Service shutdown failed.")
	} else {
		logger.Info().Msg("Service shut down gracefully.")
	}
}

// setupPubsubSubscription ensures the subscription exists and is configured with a DLQ, using v2-idiomatic calls.
func setupPubsubSubscription(ctx context.Context, client *pubsub.Client, cfg *cmd.AppConfig, logger zerolog.Logger) error {
	subAdminClient := client.SubscriptionAdminClient
	topicAdminClient := client.TopicAdminClient
	subName := fmt.Sprintf("projects/%s/subscriptions/%s", cfg.ProjectID, cfg.SubscriptionID)

	// Idempotent check: Try to get the subscription.
	_, err := subAdminClient.GetSubscription(ctx, &pubsubpb.GetSubscriptionRequest{Subscription: subName})
	if err == nil {
		logger.Info().Str("subscription_id", cfg.SubscriptionID).Msg("Pub/Sub subscription already exists.")
		return nil
	}
	if status.Code(err) != codes.NotFound {
		return fmt.Errorf("failed to check for subscription existence: %w", err)
	}

	// --- Subscription does not exist, so create it ---
	logger.Info().Str("subscription_id", cfg.SubscriptionID).Msg("Subscription not found, attempting to create it...")

	// This logic assumes a naming convention: subscription "X-sub" comes from topic "X".
	topicID := strings.TrimSuffix(cfg.SubscriptionID, "-sub")
	topicName := fmt.Sprintf("projects/%s/topics/%s", cfg.ProjectID, topicID)

	// Ensure the source topic exists.
	if _, err := topicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName}); err != nil {
		if status.Code(err) != codes.AlreadyExists {
			return fmt.Errorf("failed to create source topic %s: %w", topicName, err)
		}
	}

	// Ensure the DLQ topic exists if configured.
	dlqTopicName := ""
	if cfg.SubscriptionDLQTopicID != "" {
		dlqTopicName = fmt.Sprintf("projects/%s/topics/%s", cfg.ProjectID, cfg.SubscriptionDLQTopicID)
		if _, err := topicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: dlqTopicName}); err != nil {
			if status.Code(err) != codes.AlreadyExists {
				return fmt.Errorf("failed to create DLQ topic %s: %w", dlqTopicName, err)
			}
		}
	}

	subConfig := &pubsubpb.Subscription{
		Name:  subName,
		Topic: topicName,
		RetryPolicy: &pubsubpb.RetryPolicy{
			MinimumBackoff: &durationpb.Duration{Seconds: 10},
		},
		AckDeadlineSeconds: 20,
	}

	if dlqTopicName != "" {
		subConfig.DeadLetterPolicy = &pubsubpb.DeadLetterPolicy{
			DeadLetterTopic:     dlqTopicName,
			MaxDeliveryAttempts: 5,
		}
	}

	_, err = subAdminClient.CreateSubscription(ctx, subConfig)
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return fmt.Errorf("failed to create subscription %s: %w", cfg.SubscriptionID, err)
	}

	logger.Info().Str("subscription_id", cfg.SubscriptionID).Msg("Successfully created Pub/Sub subscription.")
	return nil
}
