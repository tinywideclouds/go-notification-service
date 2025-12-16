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

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestUpdateConfigWithEnvOverrides(t *testing.T) {
	logger := newTestLogger()

	baseConfig := func() *config.Config {
		return &config.Config{
			ProjectID:          "base-project",
			ListenAddr:         ":8080",
			SubscriptionID:     "base-sub",
			NumPipelineWorkers: 2,
			Vapid: config.VapidConfig{
				PublicKey:  "base-pub",
				PrivateKey: "base-priv",
			},
		}
	}

	t.Run("Success - All overrides applied", func(t *testing.T) {
		cfg := baseConfig()

		t.Setenv("PROJECT_ID", "env-project")
		t.Setenv("PORT", "9090")
		t.Setenv("SUBSCRIPTION_ID", "env-sub")

		// ✅ Test VAPID Overrides
		t.Setenv("VAPID_PUBLIC_KEY", "env-pub")
		t.Setenv("VAPID_PRIVATE_KEY", "env-priv")
		t.Setenv("VAPID_SUB_EMAIL", "env@test.com")

		finalCfg, err := config.UpdateConfigWithEnvOverrides(cfg, logger)
		require.NoError(t, err)

		assert.Equal(t, "env-project", finalCfg.ProjectID)
		assert.Equal(t, ":9090", finalCfg.ListenAddr)
		assert.Equal(t, "env-sub", finalCfg.SubscriptionID)

		assert.Equal(t, "env-pub", finalCfg.Vapid.PublicKey)
		assert.Equal(t, "env-priv", finalCfg.Vapid.PrivateKey)
		assert.Equal(t, "env@test.com", finalCfg.Vapid.SubscriberEmail)
	})

	t.Run("Success - Defaults preserved", func(t *testing.T) {
		cfg := baseConfig()
		finalCfg, err := config.UpdateConfigWithEnvOverrides(cfg, logger)
		require.NoError(t, err)

		assert.Equal(t, "base-project", finalCfg.ProjectID)
		assert.Equal(t, "base-pub", finalCfg.Vapid.PublicKey)
	})

	t.Run("Validation Failure - Missing ProjectID", func(t *testing.T) {
		cfg := &config.Config{SubscriptionID: "sub"}
		os.Unsetenv("PROJECT_ID")
		_, err := config.UpdateConfigWithEnvOverrides(cfg, logger)
		assert.Error(t, err)
	})
}
