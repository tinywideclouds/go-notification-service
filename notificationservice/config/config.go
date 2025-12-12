// --- File: notificationservice/config/config.go ---
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/tinywideclouds/go-microservice-base/pkg/middleware"
)

// Config defines the *single*, authoritative configuration for the Notification Service.
type Config struct {
	ProjectID              string
	ListenAddr             string
	SubscriptionID         string
	SubscriptionDLQTopicID string
	NumPipelineWorkers     int

	CorsConfig middleware.CorsConfig

	// PubsubConsumerConfig is derived from SubscriptionID
	PubsubConsumerConfig *messagepipeline.GooglePubsubConsumerConfig
}

// UpdateConfigWithEnvOverrides takes the base configuration (created from YAML)
// and completes it by applying environment variables and final validation.
func UpdateConfigWithEnvOverrides(cfg *Config, logger *slog.Logger) (*Config, error) {
	logger.Debug("Applying environment variable overrides...")

	// 1. Apply Environment Overrides
	if val := os.Getenv("PROJECT_ID"); val != "" {
		logger.Debug("Overriding config value", "key", "PROJECT_ID", "source", "env")
		cfg.ProjectID = val
	}
	if val := os.Getenv("PORT"); val != "" {
		logger.Debug("Overriding config value", "key", "PORT", "source", "env")
		cfg.ListenAddr = ":" + val
	}
	if val := os.Getenv("SUBSCRIPTION_ID"); val != "" {
		logger.Debug("Overriding config value", "key", "SUBSCRIPTION_ID", "source", "env")
		cfg.SubscriptionID = val
		// If subscription ID changes, we must regenerate the consumer config
		cfg.PubsubConsumerConfig = messagepipeline.NewGooglePubsubConsumerDefaults(val)
	}
	if val := os.Getenv("SUBSCRIPTION_DLQ_TOPIC_ID"); val != "" {
		logger.Debug("Overriding config value", "key", "SUBSCRIPTION_DLQ_TOPIC_ID", "source", "env")
		cfg.SubscriptionDLQTopicID = val
	}
	if val := os.Getenv("NUM_PIPELINE_WORKERS"); val != "" {
		if workers, err := strconv.Atoi(val); err == nil && workers > 0 {
			logger.Debug("Overriding config value", "key", "NUM_PIPELINE_WORKERS", "source", "env")
			cfg.NumPipelineWorkers = workers
		}
	}

	if corsOrigins := os.Getenv("CORS_ALLOWED_ORIGINS"); corsOrigins != "" {
		logger.Debug("Overriding config value", "key", "CORS_ALLOWED_ORIGINS", "source", "env")
		// Split by comma and trim spaces
		rawOrigins := strings.Split(corsOrigins, ",")
		var cleanOrigins []string
		for _, o := range rawOrigins {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				cleanOrigins = append(cleanOrigins, trimmed)
			}
		}
		cfg.CorsConfig.AllowedOrigins = cleanOrigins
	}

	// 2. Final Validation
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required (set via YAML or PROJECT_ID env var)")
	}
	if cfg.SubscriptionID == "" {
		return nil, fmt.Errorf("subscription_id is required (set via YAML or SUBSCRIPTION_ID env var)")
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080" // Default fallback
	}
	if cfg.NumPipelineWorkers <= 0 {
		cfg.NumPipelineWorkers = 1 // Safe default
	}

	// 3. Ensure Consumer Config is present (if not set in YAML or Env, but ID exists)
	if cfg.PubsubConsumerConfig == nil && cfg.SubscriptionID != "" {
		cfg.PubsubConsumerConfig = messagepipeline.NewGooglePubsubConsumerDefaults(cfg.SubscriptionID)
	}

	logger.Debug("Configuration finalized and validated successfully")
	return cfg, nil
}
