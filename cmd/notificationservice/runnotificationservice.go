// --- File: cmd/notificationservice/runnotificationservice.go ---
package main

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"

	firebase "firebase.google.com/go/v4"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/tinywideclouds/go-microservice-base/pkg/middleware"

	"github.com/tinywideclouds/go-notification-service/internal/platform/fcm"
	"github.com/tinywideclouds/go-notification-service/internal/platform/web"

	"github.com/tinywideclouds/go-notification-service/internal/storage/cache"
	fsStore "github.com/tinywideclouds/go-notification-service/internal/storage/firestore"
	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"

	"github.com/tinywideclouds/go-notification-service/notificationservice"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

//go:embed local.yaml
var configFile []byte

func main() {
	var logLevel slog.Level
	switch os.Getenv("LOG_LEVEL") {
	case "debug", "DEBUG":
		logLevel = slog.LevelDebug
	case "info", "INFO":
		logLevel = slog.LevelInfo
	case "warn", "WARN":
		logLevel = slog.LevelWarn
	case "error", "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})).With("service", "go-notifications-service")
	slog.SetDefault(logger)

	ctx := context.Background()

	// --- Config Loading ---
	var yamlCfg config.YamlConfig
	if err := yaml.Unmarshal(configFile, &yamlCfg); err != nil {
		logger.Error("Failed to unmarshal embedded yaml config", "err", err)
		os.Exit(1)
	}
	baseCfg, _ := config.NewConfigFromYaml(&yamlCfg, logger)
	cfg, err := config.UpdateConfigWithEnvOverrides(baseCfg, logger)
	if err != nil {
		logger.Error("Config failed", "err", err)
		os.Exit(1)
	}

	// --- Infrastructure Clients ---
	psClient, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		logger.Error("PubSub client failed", "err", err)
		os.Exit(1)
	}
	defer psClient.Close()

	fsClient, err := firestore.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		logger.Error("Firestore client failed", "err", err)
		os.Exit(1)
	}
	defer fsClient.Close()

	// --- Token Store (Decorated) ---
	var tokenStore dispatch.TokenStore = fsStore.NewFirestoreStore(fsClient)
	logger.Info("TokenStore initialized", "type", "firestore")

	if cfg.Redis.Enabled {
		logger.Info("Initializing Redis Cache layer...", "addr", cfg.Redis.Addr)
		redisClient, err := cache.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			logger.Error("Failed to connect to Redis", "err", err)
			os.Exit(1)
		}
		defer redisClient.Close()
		tokenStore = cache.NewCachedTokenStore(tokenStore, redisClient, 24*time.Hour)
		logger.Info("TokenStore upgraded", "type", "redis_cached_firestore")
	}

	// --- Auth ---
	identityURL := os.Getenv("IDENTITY_SERVICE_URL")
	if identityURL == "" {
		identityURL = "http://localhost:3000"
	}
	jwksURL, _ := middleware.DiscoverAndValidateJWTConfig(identityURL, middleware.RSA256, logger)
	authMiddleware, _ := middleware.NewJWKSAuthMiddleware(jwksURL, logger)

	// --- Dispatchers ---

	// A. Mobile (FCM)
	fbApp, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: cfg.ProjectID})
	if err != nil {
		logger.Error("Failed to initialize Firebase App", "err", err)
		os.Exit(1)
	}
	fcmMessaging, err := fbApp.Messaging(ctx)
	if err != nil {
		logger.Error("Failed to create FCM messaging client", "err", err)
		os.Exit(1)
	}
	fcmDispatcher := fcm.NewDispatcher(fcmMessaging, logger)

	// B. Web (VAPID) - ✅ Using Config Logic
	// Fail fast if keys are missing but web support is expected?
	// We'll warn for now to allow partial deployment (e.g. mobile only).
	if cfg.Vapid.PrivateKey == "" || cfg.Vapid.PublicKey == "" {
		logger.Warn("VAPID keys missing in configuration. Web Push will fail.")
	} else {
		logger.Info("Web Dispatcher enabled", "public_key", cfg.Vapid.PublicKey)
	}
	// We pass the keys from config
	webDispatcher := web.NewDispatcher(cfg.Vapid, logger)

	// --- Consumer & Service ---
	consumer, _ := newIngestionConsumer(ctx, cfg, psClient, logger)

	service, err := notificationservice.New(
		cfg,
		consumer,
		fcmDispatcher,
		webDispatcher,
		tokenStore,
		authMiddleware,
		logger,
	)
	if err != nil {
		logger.Error("Service creation failed", "err", err)
		os.Exit(1)
	}

	logger.Info("Starting service...")
	if err := service.Start(ctx); err != nil {
		logger.Error("Service shutdown with error", "err", err)
		os.Exit(1)
	}
}

// ... (Helpers remain unchanged) ...
func newIngestionConsumer(ctx context.Context, cfg *config.Config, psClient *pubsub.Client, logger *slog.Logger) (messagepipeline.MessageConsumer, error) {
	sub := convertPubsub(cfg.ProjectID, cfg.PubsubConsumerConfig.SubscriptionID, "subscriptions")
	topicID := convertPubsub(cfg.ProjectID, cfg.TopicID, "topics")
	dlt := convertPubsub(cfg.ProjectID, cfg.SubscriptionDLQTopicID, "topics")

	subConfig := &pubsubpb.Subscription{
		Name:               sub,
		Topic:              topicID,
		AckDeadlineSeconds: 10,
		DeadLetterPolicy: &pubsubpb.DeadLetterPolicy{
			DeadLetterTopic:     dlt,
			MaxDeliveryAttempts: 5,
		},
		EnableMessageOrdering: false,
	}
	logger.Debug("Ensuring subscription exists", "sub", subConfig.Name, "topic", subConfig.Topic)
	_, err := psClient.SubscriptionAdminClient.CreateSubscription(ctx, subConfig)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			logger.Debug("Subscription already exists, skipping creation", "sub", subConfig.Name)
		} else {
			logger.Error("Failed to create subscription", "sub", subConfig.Name, "err", err)
			return nil, fmt.Errorf("could not create sub: %s", sub)
		}
	}

	return messagepipeline.NewGooglePubsubConsumer(
		messagepipeline.NewGooglePubsubConsumerDefaults(subConfig.Name), psClient, logger,
	)
}

type PS string

func convertPubsub(project, id string, ps PS) string {
	return fmt.Sprintf("projects/%s/%s/%s", project, ps, id)
}
