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

type RedisConfig struct {
	Enabled  bool
	Addr     string
	Password string
	DB       int
}

type VapidConfig struct {
	PublicKey       string
	PrivateKey      string
	SubscriberEmail string
}

// Config defines the *single*, authoritative configuration.
type Config struct {
	ProjectID              string
	ListenAddr             string
	SubscriptionID         string
	SubscriptionDLQTopicID string
	NumPipelineWorkers     int

	CorsConfig middleware.CorsConfig
	Redis      RedisConfig
	Vapid      VapidConfig // ✅ Added

	TopicID              string
	PubsubConsumerConfig *messagepipeline.GooglePubsubConsumerConfig
}

// UpdateConfigWithEnvOverrides applies environment variables and final validation.
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

	// Redis Overrides
	if val := os.Getenv("REDIS_ADDR"); val != "" {
		cfg.Redis.Addr = val
		cfg.Redis.Enabled = true
	}
	if val := os.Getenv("REDIS_PASSWORD"); val != "" {
		cfg.Redis.Password = val
	}
	if val := os.Getenv("REDIS_DB"); val != "" {
		if db, err := strconv.Atoi(val); err == nil {
			cfg.Redis.DB = db
		}
	}
	if val := os.Getenv("REDIS_ENABLED"); val != "" {
		enabled, _ := strconv.ParseBool(val)
		cfg.Redis.Enabled = enabled
	}

	// ✅ VAPID Overrides
	if val := os.Getenv("VAPID_PUBLIC_KEY"); val != "" {
		logger.Debug("Overriding config value", "key", "VAPID_PUBLIC_KEY", "source", "env")
		cfg.Vapid.PublicKey = val
	}
	if val := os.Getenv("VAPID_PRIVATE_KEY"); val != "" {
		logger.Debug("Overriding config value", "key", "VAPID_PRIVATE_KEY", "source", "env")
		cfg.Vapid.PrivateKey = val
	}
	if val := os.Getenv("VAPID_SUB_EMAIL"); val != "" {
		logger.Debug("Overriding config value", "key", "VAPID_SUB_EMAIL", "source", "env")
		cfg.Vapid.SubscriberEmail = val
	}

	// CORS Overrides
	if corsOrigins := os.Getenv("CORS_ALLOWED_ORIGINS"); corsOrigins != "" {
		logger.Debug("Overriding config value", "key", "CORS_ALLOWED_ORIGINS", "source", "env")
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
		cfg.ListenAddr = ":8080"
	}
	if cfg.NumPipelineWorkers <= 0 {
		cfg.NumPipelineWorkers = 1
	}

	if cfg.PubsubConsumerConfig == nil && cfg.SubscriptionID != "" {
		cfg.PubsubConsumerConfig = messagepipeline.NewGooglePubsubConsumerDefaults(cfg.SubscriptionID)
	}

	logger.Debug("Configuration finalized and validated successfully")
	return cfg, nil
}
