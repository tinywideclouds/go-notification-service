// --- File: notificationservice/config/yaml_config_test.go ---
package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-microservice-base/pkg/middleware"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"
)

func TestNewConfigFromYaml(t *testing.T) {
	logger := newTestLogger()

	t.Run("Success - maps all fields correctly", func(t *testing.T) {
		yamlCfg := &config.YamlConfig{
			ProjectID:              "yaml-project",
			ListenAddr:             ":9000",
			TopicID:                "yaml-topic",
			SubscriptionID:         "yaml-subscription",
			SubscriptionDLQTopicID: "yaml-dlq",
			NumPipelineWorkers:     5,
			CorsConfig: config.YamlCorsConfig{
				AllowedOrigins: []string{"http://yaml.com"},
				Role:           "editor",
			},
			// ✅ Test VAPID Mapping
			VapidConfig: config.YamlVapidConfig{
				PublicKey:       "yaml-public-key",
				PrivateKey:      "yaml-private-key",
				SubscriberEmail: "yaml@test.com",
			},
		}

		cfg, err := config.NewConfigFromYaml(yamlCfg, logger)

		require.NoError(t, err)
		require.NotNil(t, cfg)

		// 1. Direct Field Mapping
		assert.Equal(t, "yaml-project", cfg.ProjectID)
		assert.Equal(t, ":9000", cfg.ListenAddr)
		assert.Equal(t, "yaml-topic", cfg.TopicID)
		assert.Equal(t, "yaml-subscription", cfg.SubscriptionID)
		assert.Equal(t, "yaml-dlq", cfg.SubscriptionDLQTopicID)
		assert.Equal(t, 5, cfg.NumPipelineWorkers)

		// 2. Complex Logic: CORS
		assert.Equal(t, []string{"http://yaml.com"}, cfg.CorsConfig.AllowedOrigins)
		assert.Equal(t, middleware.CorsRoleEditor, cfg.CorsConfig.Role)

		// 3. ✅ Verify VAPID
		assert.Equal(t, "yaml-public-key", cfg.Vapid.PublicKey)
		assert.Equal(t, "yaml-private-key", cfg.Vapid.PrivateKey)
		assert.Equal(t, "yaml@test.com", cfg.Vapid.SubscriberEmail)

		assert.NotNil(t, cfg.PubsubConsumerConfig)
	})

	t.Run("Success - Handles missing optional fields gracefully", func(t *testing.T) {
		yamlCfg := &config.YamlConfig{
			ProjectID:      "minimal-project",
			SubscriptionID: "minimal-sub",
		}

		cfg, err := config.NewConfigFromYaml(yamlCfg, logger)

		require.NoError(t, err)
		assert.Equal(t, "minimal-project", cfg.ProjectID)
		assert.Equal(t, 0, cfg.NumPipelineWorkers)
		assert.Empty(t, cfg.ListenAddr)
		assert.Empty(t, cfg.Vapid.PublicKey) // Verify zero value
	})
}
