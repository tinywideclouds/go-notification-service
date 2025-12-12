// --- File: cmd/notificationservice/runnotificationservice.go ---
package main

import (
	"context"
	_ "embed"
	"log/slog"
	"os"

	"cloud.google.com/go/firestore" // <-- ADDED
	"cloud.google.com/go/pubsub/v2"
	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/rs/zerolog"
	"github.com/tinywideclouds/go-microservice-base/pkg/middleware" // <-- ADDED
	"github.com/tinywideclouds/go-notification-service/internal/platform/apns"
	"github.com/tinywideclouds/go-notification-service/internal/platform/fcm"
	fsStore "github.com/tinywideclouds/go-notification-service/internal/storage/firestore" // <-- ADDED
	"github.com/tinywideclouds/go-notification-service/notificationservice"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"
	"github.com/tinywideclouds/go-notification-service/pkg/dispatch"
	"gopkg.in/yaml.v3"
)

//go:embed local.yaml
var configFile []byte

func main() {
	// ... (Logging setup same as before) ...
	logLevel := slog.LevelInfo // Simplified for brevity
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)
	ctx := context.Background()

	// ... (Config loading same as before) ...
	var yamlCfg config.YamlConfig
	yaml.Unmarshal(configFile, &yamlCfg)
	baseCfg, _ := config.NewConfigFromYaml(&yamlCfg, logger)
	cfg, err := config.UpdateConfigWithEnvOverrides(baseCfg, logger)
	if err != nil {
		logger.Error("Config failed", "err", err)
		os.Exit(1)
	}

	// --- Dependencies ---

	// 1. PubSub
	psClient, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		logger.Error("PubSub client failed", "err", err)
		os.Exit(1)
	}
	defer psClient.Close()

	// 2. Firestore (Token Store) <-- NEW
	fsClient, err := firestore.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		logger.Error("Firestore client failed", "err", err)
		os.Exit(1)
	}
	defer fsClient.Close()

	tokenStore := fsStore.NewTokenStore(fsClient, "device-tokens", logger)

	// 3. Auth Middleware <-- NEW (Needed for Registration API)
	// We need an Identity Service URL for this.
	// Since base config didn't have it, we might need to add it to Config struct
	// or assume env var. For now, let's assume standard discovery.
	identityURL := os.Getenv("IDENTITY_SERVICE_URL")
	if identityURL == "" {
		identityURL = "http://localhost:3000" // Fallback
	}
	jwksURL, _ := middleware.DiscoverAndValidateJWTConfig(identityURL, middleware.RSA256, logger)
	authMiddleware, _ := middleware.NewJWKSAuthMiddleware(jwksURL, logger)

	// 4. Dispatchers & Consumer
	fcmDispatcher, _ := fcm.NewLoggingDispatcher(logger)
	apnsDispatcher, _ := apns.NewLoggingDispatcher(logger)
	dispatchers := map[string]dispatch.Dispatcher{
		"android": fcmDispatcher,
		"ios":     apnsDispatcher,
		"web":     fcmDispatcher, // Angular PWA will use FCM
	}

	consumer, _ := messagepipeline.NewGooglePubsubConsumer(cfg.PubsubConsumerConfig, psClient, zerolog.Nop())

	// --- Service ---
	service, err := notificationservice.New(
		cfg,
		consumer,
		dispatchers,
		tokenStore,     // Injected
		authMiddleware, // Injected
		logger,
	)
	if err != nil {
		logger.Error("Service creation failed", "err", err)
		os.Exit(1)
	}

	// ... (Startup/Shutdown logic same as before) ...
	service.Start(ctx) // etc...
}
