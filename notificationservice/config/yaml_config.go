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

type YamlRedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	Enabled  bool   `yaml:"enabled"`
}

type YamlVapidConfig struct {
	PublicKey       string `yaml:"public_key"`
	PrivateKey      string `yaml:"private_key"`
	SubscriberEmail string `yaml:"subscriber_email"`
}

// YamlConfig is the structure that mirrors the raw config.yaml file.
type YamlConfig struct {
	ProjectID              string          `yaml:"project_id"`
	ListenAddr             string          `yaml:"listen_addr"`
	SubscriberEmail        string          `yaml:"subscriber_email"`
	TopicID                string          `yaml:"topic_id"`
	SubscriptionID         string          `yaml:"subscription_id"`
	SubscriptionDLQTopicID string          `yaml:"subscription_dlq_topic_id"`
	CorsConfig             YamlCorsConfig  `yaml:"cors"`
	RedisConfig            YamlRedisConfig `yaml:"redis"`
	VapidConfig            YamlVapidConfig `yaml:"vapid"` // ✅ Added
	NumPipelineWorkers     int             `yaml:"num_pipeline_workers"`
}

// NewConfigFromYaml converts the YamlConfig into a clean, base Config struct.
func NewConfigFromYaml(baseCfg *YamlConfig, logger *slog.Logger) (*Config, error) {
	logger.Debug("Mapping YAML config to base config struct")

	cfg := &Config{
		ProjectID:      baseCfg.ProjectID,
		ListenAddr:     baseCfg.ListenAddr,
		TopicID:        baseCfg.TopicID,
		SubscriptionID: baseCfg.SubscriptionID,
		CorsConfig: middleware.CorsConfig{
			AllowedOrigins: baseCfg.CorsConfig.AllowedOrigins,
			Role:           middleware.CorsRole(baseCfg.CorsConfig.Role),
		},
		Redis: RedisConfig{
			Addr:     baseCfg.RedisConfig.Addr,
			Password: baseCfg.RedisConfig.Password,
			DB:       baseCfg.RedisConfig.DB,
			Enabled:  baseCfg.RedisConfig.Enabled,
		},
		Vapid: VapidConfig{ // ✅ Map Vapid
			PublicKey:       baseCfg.VapidConfig.PublicKey,
			PrivateKey:      baseCfg.VapidConfig.PrivateKey,
			SubscriberEmail: baseCfg.VapidConfig.SubscriberEmail,
		},
		SubscriptionDLQTopicID: baseCfg.SubscriptionDLQTopicID,
		NumPipelineWorkers:     baseCfg.NumPipelineWorkers,
	}

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
