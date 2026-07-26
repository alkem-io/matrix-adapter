package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Point CONFIG_PATH to a non-existent file so no YAML is loaded.
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.yaml"))
	t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "") // isolate from ambient env (now a parse error)

	// Clear env vars that loadEnvVars would pick up.
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("LOGGING_LEVEL_CONSOLE", "")
	t.Setenv("SYNAPSE_SERVER_URL", "")
	t.Setenv("SYNAPSE_HOMESERVER_NAME", "")
	t.Setenv("MATRIX_AS_TOKEN", "")
	t.Setenv("MATRIX_HS_TOKEN", "")
	t.Setenv("MATRIX_BOT_ACTOR_ID", "")
	t.Setenv("MATRIX_BOT_DISPLAY_NAME", "")
	t.Setenv("SYNAPSE_SERVER_SHARED_SECRET", "")
	t.Setenv("SYNAPSE_REGISTRATION_SECRET", "")
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_HOST", "")
	t.Setenv("RABBITMQ_PORT", "")
	t.Setenv("RABBITMQ_USER", "")
	t.Setenv("RABBITMQ_PASSWORD", "")
	t.Setenv("FILE_SERVICE_URL", "")
	t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "development", cfg.App.Environment)
	assert.Equal(t, "info", cfg.App.LogLevel)
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", cfg.Matrix.BotActorID)
	assert.Equal(t, "Alkemio", cfg.Matrix.BotDisplayName)
	assert.Empty(t, cfg.Matrix.HomeserverURL)
	assert.Empty(t, cfg.Matrix.HomeserverName)
	assert.Empty(t, cfg.Matrix.AppServiceToken)
	assert.Empty(t, cfg.Matrix.HomeserverToken)
	assert.Empty(t, cfg.Matrix.RegistrationSecret)
	assert.Empty(t, cfg.RabbitMQ.URL)
	assert.Empty(t, cfg.FileService.URL, "file-service is optional for text-only deployments")
}

func TestLoad_MatrixEnvOverrides(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.yaml"))
	t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "") // isolate from ambient env (now a parse error)

	t.Setenv("SYNAPSE_SERVER_URL", "https://matrix.test")
	t.Setenv("SYNAPSE_HOMESERVER_NAME", "test.server")
	t.Setenv("MATRIX_AS_TOKEN", "as-tok-123")
	t.Setenv("MATRIX_HS_TOKEN", "hs-tok-456")
	t.Setenv("MATRIX_BOT_ACTOR_ID", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	t.Setenv("MATRIX_BOT_DISPLAY_NAME", "TestBot")

	// Clear unrelated vars.
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("LOGGING_LEVEL_CONSOLE", "")
	t.Setenv("SYNAPSE_SERVER_SHARED_SECRET", "")
	t.Setenv("SYNAPSE_REGISTRATION_SECRET", "")
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_HOST", "")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "https://matrix.test", cfg.Matrix.HomeserverURL)
	assert.Equal(t, "test.server", cfg.Matrix.HomeserverName)
	assert.Equal(t, "as-tok-123", cfg.Matrix.AppServiceToken)
	assert.Equal(t, "hs-tok-456", cfg.Matrix.HomeserverToken)
	assert.Equal(t, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", cfg.Matrix.BotActorID)
	assert.Equal(t, "TestBot", cfg.Matrix.BotDisplayName)
}

func TestLoad_FileServiceEnvOverrides(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.yaml"))
	t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "") // isolate from ambient env (now a parse error)
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_HOST", "")

	t.Run("URL and max attachment bytes", func(t *testing.T) {
		t.Setenv("FILE_SERVICE_URL", "http://file-service:4000")
		t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "1048576")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "http://file-service:4000", cfg.FileService.URL)
		assert.Equal(t, int64(1048576), cfg.FileService.MaxAttachmentBytes)
	})

	t.Run("invalid max attachment bytes fails config loading", func(t *testing.T) {
		t.Setenv("FILE_SERVICE_URL", "http://file-service:4000")
		t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "not-a-number")

		cfg, err := Load()
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "invalid FILE_SERVICE_MAX_ATTACHMENT_BYTES")
		assert.Contains(t, err.Error(), "not-a-number")
	})
}

func TestLoad_RegistrationSecretPrecedence(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.yaml"))
	t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "") // isolate from ambient env (now a parse error)
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_HOST", "")

	t.Run("only SYNAPSE_SERVER_SHARED_SECRET", func(t *testing.T) {
		t.Setenv("SYNAPSE_SERVER_SHARED_SECRET", "shared-secret")
		t.Setenv("SYNAPSE_REGISTRATION_SECRET", "")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "shared-secret", cfg.Matrix.RegistrationSecret)
	})

	t.Run("only SYNAPSE_REGISTRATION_SECRET", func(t *testing.T) {
		t.Setenv("SYNAPSE_SERVER_SHARED_SECRET", "")
		t.Setenv("SYNAPSE_REGISTRATION_SECRET", "reg-secret")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "reg-secret", cfg.Matrix.RegistrationSecret)
	})

	t.Run("both set — SYNAPSE_REGISTRATION_SECRET wins", func(t *testing.T) {
		t.Setenv("SYNAPSE_SERVER_SHARED_SECRET", "shared-secret")
		t.Setenv("SYNAPSE_REGISTRATION_SECRET", "reg-secret")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "reg-secret", cfg.Matrix.RegistrationSecret,
			"SYNAPSE_REGISTRATION_SECRET should take precedence over SYNAPSE_SERVER_SHARED_SECRET")
	})
}

func TestLoad_RabbitMQ_FullURL(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.yaml"))
	t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "") // isolate from ambient env (now a parse error)
	t.Setenv("SYNAPSE_SERVER_SHARED_SECRET", "")
	t.Setenv("SYNAPSE_REGISTRATION_SECRET", "")

	t.Setenv("RABBITMQ_URL", "amqp://user:pass@rabbit.host:5672/")
	// Individual components should be ignored when URL is set.
	t.Setenv("RABBITMQ_HOST", "ignored.host")
	t.Setenv("RABBITMQ_PORT", "9999")
	t.Setenv("RABBITMQ_USER", "ignored")
	t.Setenv("RABBITMQ_PASSWORD", "ignored")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "amqp://user:pass@rabbit.host:5672/", cfg.RabbitMQ.URL)
}

func TestLoad_RabbitMQ_IndividualComponents(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.yaml"))
	t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "") // isolate from ambient env (now a parse error)
	t.Setenv("SYNAPSE_SERVER_SHARED_SECRET", "")
	t.Setenv("SYNAPSE_REGISTRATION_SECRET", "")
	t.Setenv("RABBITMQ_URL", "")

	t.Run("host with user and password", func(t *testing.T) {
		t.Setenv("RABBITMQ_HOST", "mq.example.com")
		t.Setenv("RABBITMQ_PORT", "5673")
		t.Setenv("RABBITMQ_USER", "admin")
		t.Setenv("RABBITMQ_PASSWORD", "s3cret")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "amqp://admin:s3cret@mq.example.com:5673/", cfg.RabbitMQ.URL)
	})

	t.Run("host without credentials uses default port", func(t *testing.T) {
		t.Setenv("RABBITMQ_HOST", "mq.example.com")
		t.Setenv("RABBITMQ_PORT", "")
		t.Setenv("RABBITMQ_USER", "")
		t.Setenv("RABBITMQ_PASSWORD", "")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "amqp://mq.example.com:5672/", cfg.RabbitMQ.URL)
	})

	t.Run("no host means empty URL", func(t *testing.T) {
		t.Setenv("RABBITMQ_HOST", "")
		t.Setenv("RABBITMQ_PORT", "")
		t.Setenv("RABBITMQ_USER", "")
		t.Setenv("RABBITMQ_PASSWORD", "")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Empty(t, cfg.RabbitMQ.URL)
	})
}

func TestLoad_YAMLConfigFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "") // isolate from ambient env (now a parse error)

	yamlContent := `
app:
  environment: production
  log_level: warn
matrix:
  homeserver_url: https://yaml.matrix.local
  homeserver_name: yaml.local
  as_token: yaml-as-token
  hs_token: yaml-hs-token
  bot_actor_id: "11111111-2222-3333-4444-555555555555"
  bot_display_name: YAMLBot
  registration_secret: yaml-reg-secret
rabbitmq:
  url: amqp://yaml-rabbit:5672/
`
	err := os.WriteFile(configFile, []byte(yamlContent), 0o600)
	require.NoError(t, err)

	t.Setenv("CONFIG_PATH", configFile)

	// Clear all env overrides so YAML values survive.
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("LOGGING_LEVEL_CONSOLE", "")
	t.Setenv("SYNAPSE_SERVER_URL", "")
	t.Setenv("SYNAPSE_HOMESERVER_NAME", "")
	t.Setenv("MATRIX_AS_TOKEN", "")
	t.Setenv("MATRIX_HS_TOKEN", "")
	t.Setenv("MATRIX_BOT_ACTOR_ID", "")
	t.Setenv("MATRIX_BOT_DISPLAY_NAME", "")
	t.Setenv("SYNAPSE_SERVER_SHARED_SECRET", "")
	t.Setenv("SYNAPSE_REGISTRATION_SECRET", "")
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("RABBITMQ_HOST", "")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "production", cfg.App.Environment)
	assert.Equal(t, "warn", cfg.App.LogLevel)
	assert.Equal(t, "https://yaml.matrix.local", cfg.Matrix.HomeserverURL)
	assert.Equal(t, "yaml.local", cfg.Matrix.HomeserverName)
	assert.Equal(t, "yaml-as-token", cfg.Matrix.AppServiceToken)
	assert.Equal(t, "yaml-hs-token", cfg.Matrix.HomeserverToken)
	assert.Equal(t, "11111111-2222-3333-4444-555555555555", cfg.Matrix.BotActorID)
	assert.Equal(t, "YAMLBot", cfg.Matrix.BotDisplayName)
	assert.Equal(t, "yaml-reg-secret", cfg.Matrix.RegistrationSecret)
	assert.Equal(t, "amqp://yaml-rabbit:5672/", cfg.RabbitMQ.URL)
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	t.Setenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES", "") // isolate from ambient env (now a parse error)

	yamlContent := `
app:
  environment: production
matrix:
  homeserver_url: https://yaml.matrix.local
rabbitmq:
  url: amqp://yaml-rabbit:5672/
`
	err := os.WriteFile(configFile, []byte(yamlContent), 0o600)
	require.NoError(t, err)

	t.Setenv("CONFIG_PATH", configFile)
	t.Setenv("ENVIRONMENT", "staging")
	t.Setenv("SYNAPSE_SERVER_URL", "https://env.matrix.local")
	t.Setenv("RABBITMQ_URL", "amqp://env-rabbit:5672/")

	// Clear others.
	t.Setenv("LOGGING_LEVEL_CONSOLE", "")
	t.Setenv("SYNAPSE_HOMESERVER_NAME", "")
	t.Setenv("MATRIX_AS_TOKEN", "")
	t.Setenv("MATRIX_HS_TOKEN", "")
	t.Setenv("MATRIX_BOT_ACTOR_ID", "")
	t.Setenv("MATRIX_BOT_DISPLAY_NAME", "")
	t.Setenv("SYNAPSE_SERVER_SHARED_SECRET", "")
	t.Setenv("SYNAPSE_REGISTRATION_SECRET", "")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "staging", cfg.App.Environment, "env var should override YAML")
	assert.Equal(t, "https://env.matrix.local", cfg.Matrix.HomeserverURL, "env var should override YAML")
	assert.Equal(t, "amqp://env-rabbit:5672/", cfg.RabbitMQ.URL, "env var should override YAML")
}
