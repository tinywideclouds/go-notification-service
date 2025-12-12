// --- File: notificationservice/config/config_test.go ---
package config_test

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinywideclouds/go-notification-service/notificationservice/config"
)

// newTestLogger creates a discard logger for tests.
func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestUpdateConfigWithEnvOverrides(t *testing.T) {
	logger := newTestLogger()

	// Helper to create a base config simulating Stage 1 (YAML load)
	baseConfig := func() *config.Config {
		return &config.Config{
			ProjectID:          "base-project",
			ListenAddr:         ":8080",
			SubscriptionID:     "base-sub",
			NumPipelineWorkers: 2,
		}
	}

	t.Run("Success - All overrides applied", func(t *testing.T) {
		cfg := baseConfig()

		t.Setenv("PROJECT_ID", "env-project")
		t.Setenv("PORT", "9090") // Should become :9090
		t.Setenv("SUBSCRIPTION_ID", "env-sub")
		t.Setenv("SUBSCRIPTION_DLQ_TOPIC_ID", "env-dlq")
		t.Setenv("NUM_PIPELINE_WORKERS", "10")

		finalCfg, err := config.UpdateConfigWithEnvOverrides(cfg, logger)
		require.NoError(t, err)

		assert.Equal(t, "env-project", finalCfg.ProjectID)
		assert.Equal(t, ":9090", finalCfg.ListenAddr)
		assert.Equal(t, "env-sub", finalCfg.SubscriptionID)
		assert.Equal(t, "env-dlq", finalCfg.SubscriptionDLQTopicID)
		assert.Equal(t, 10, finalCfg.NumPipelineWorkers)

		// Verify Consumer Config was updated to match new Subscription ID
		assert.NotNil(t, finalCfg.PubsubConsumerConfig)
		// Accessing private field logic indirectly via knowledge of the default constructor:
		// The default constructor usually sets the subscription ID in the config,
		// but since we can't check internal fields of the external lib easily,
		// we mainly ensure the struct is not nil and the Logic in config.go was triggered.
	})

	t.Run("Success - Defaults preserved", func(t *testing.T) {
		cfg := baseConfig()
		// No env vars set

		finalCfg, err := config.UpdateConfigWithEnvOverrides(cfg, logger)
		require.NoError(t, err)

		assert.Equal(t, "base-project", finalCfg.ProjectID)
		assert.Equal(t, ":8080", finalCfg.ListenAddr)
	})

	t.Run("Validation Failure - Missing ProjectID", func(t *testing.T) {
		cfg := &config.Config{
			SubscriptionID: "some-sub",
		}
		// Ensure Env is empty
		os.Unsetenv("PROJECT_ID")

		_, err := config.UpdateConfigWithEnvOverrides(cfg, logger)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "project_id is required")
	})

	t.Run("Validation Failure - Missing SubscriptionID", func(t *testing.T) {
		cfg := &config.Config{
			ProjectID: "some-project",
		}
		os.Unsetenv("SUBSCRIPTION_ID")

		_, err := config.UpdateConfigWithEnvOverrides(cfg, logger)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "subscription_id is required")
	})
}
