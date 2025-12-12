// --- File: notificationservice/config/yaml_config.go ---
package config

import (
	"log/slog"

	"github.com/illmade-knight/go-dataflow/pkg/messagepipeline"
	"github.com/tinywideclouds/go-microservice-base/pkg/middleware"
)

type YamlCorsConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	Role           string   `yaml:"role"`
}

// YamlConfig is the structure that mirrors the raw config.yaml file.
type YamlConfig struct {
	ProjectID              string         `yaml:"project_id"`
	ListenAddr             string         `yaml:"listen_addr"`
	SubscriptionID         string         `yaml:"subscription_id"`
	SubscriptionDLQTopicID string         `yaml:"subscription_dlq_topic_id"`
	CorsConfig             YamlCorsConfig `yaml:"cors"`
	NumPipelineWorkers     int            `yaml:"num_pipeline_workers"`
}

// NewConfigFromYaml converts the YamlConfig into a clean, base Config struct.
// This struct is the "Stage 1" configuration, ready to be augmented by environment overrides.
func NewConfigFromYaml(baseCfg *YamlConfig, logger *slog.Logger) (*Config, error) {
	logger.Debug("Mapping YAML config to base config struct")

	// Map and Build initial Config structure
	cfg := &Config{
		ProjectID:      baseCfg.ProjectID,
		ListenAddr:     baseCfg.ListenAddr,
		SubscriptionID: baseCfg.SubscriptionID,
		CorsConfig: middleware.CorsConfig{
			AllowedOrigins: baseCfg.CorsConfig.AllowedOrigins,
			Role:           middleware.CorsRole(baseCfg.CorsConfig.Role),
		},
		SubscriptionDLQTopicID: baseCfg.SubscriptionDLQTopicID,
		NumPipelineWorkers:     baseCfg.NumPipelineWorkers,
	}

	// Initialize the PubsubConsumerConfig based on the loaded SubscriptionID
	// This might get updated in Stage 2 if the SubscriptionID is overridden by env vars.
	if cfg.SubscriptionID != "" {
		cfg.PubsubConsumerConfig = messagepipeline.NewGooglePubsubConsumerDefaults(cfg.SubscriptionID)
	}

	logger.Debug("YAML config mapping complete",
		"project_id", cfg.ProjectID,
		"listen_addr", cfg.ListenAddr,
		"subscription_id", cfg.SubscriptionID,
	)

	return cfg, nil
}
