// Package config handles configuration loading from files and environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// Config holds the application configuration.
type Config struct {
	App struct {
		Environment string `yaml:"environment"`
		LogLevel    string `yaml:"log_level"`
	} `yaml:"app"`

	Matrix struct {
		HomeserverURL      string `yaml:"homeserver_url"`
		HomeserverName     string `yaml:"homeserver_name"`
		AppServiceToken    string `yaml:"as_token"`
		HomeserverToken    string `yaml:"hs_token"`
		BotActorID         string `yaml:"bot_actor_id"`        // Bot's Alkemio UUID, used as Matrix localpart
		BotDisplayName     string `yaml:"bot_display_name"`    // Display name for the bot user in Matrix
		RegistrationSecret string `yaml:"registration_secret"` // Synapse registration_shared_secret for admin bootstrap
	} `yaml:"matrix"`

	RabbitMQ struct {
		URL string `yaml:"url"`
	} `yaml:"rabbitmq"`

	FileService struct {
		// URL is the internal base URL of the Alkemio file-service
		// (cluster-only). Used to fetch document bytes for outbound media
		// attachments, e.g. GET {URL}/internal/file/{id}/content.
		URL string `yaml:"url"`
		// MaxAttachmentBytes caps how many bytes are read from file-service for a
		// single outbound attachment. <= 0 means use the built-in default (50 MiB).
		MaxAttachmentBytes int64 `yaml:"max_attachment_bytes"`
	} `yaml:"file_service"`

	Send struct {
		// TimeoutSeconds bounds a single message-send (text + fetch + up to 10
		// uploads). One slow/stuck send can't block the send worker indefinitely.
		// <= 0 means use the built-in default.
		TimeoutSeconds int `yaml:"timeout_seconds"`
		// Concurrency is the number of send workers draining the message-send
		// topic. Sends are partitioned by room, so distinct rooms progress
		// concurrently while same-room sends stay strictly ordered. <= 0 means
		// use the built-in default.
		Concurrency int `yaml:"concurrency"`
	} `yaml:"send"`
}

// Send tuning defaults (applied when the corresponding config value is <= 0).
const (
	// DefaultSendTimeoutSeconds bounds a single send: a media send does a
	// file-service fetch plus up to 10 uploads, which can legitimately take tens
	// of seconds, but must not stall its worker for minutes.
	DefaultSendTimeoutSeconds = 120
	// DefaultSendConcurrency is the number of room-partitioned send workers.
	DefaultSendConcurrency = 8
)

// File-service attachment memory defaults (applied when the corresponding config
// value is <= 0).
const (
	// DefaultMaxAttachmentBytes caps how many bytes are read from file-service for
	// a single outbound attachment (50 MiB).
	DefaultMaxAttachmentBytes int64 = 50 * 1024 * 1024
)

// MaxAttachmentBytes returns the configured per-attachment byte cap, falling back
// to DefaultMaxAttachmentBytes when unset or non-positive. Nil-safe so callers
// need not guard a nil *Config.
func (c *Config) MaxAttachmentBytes() int64 {
	if c != nil && c.FileService.MaxAttachmentBytes > 0 {
		return c.FileService.MaxAttachmentBytes
	}
	return DefaultMaxAttachmentBytes
}

// SendTimeoutSeconds returns the configured per-send timeout, falling back to
// the built-in default when unset or non-positive.
func (c *Config) SendTimeoutSeconds() int {
	if c.Send.TimeoutSeconds > 0 {
		return c.Send.TimeoutSeconds
	}
	return DefaultSendTimeoutSeconds
}

// SendConcurrency returns the configured number of send workers, falling back
// to the built-in default when unset or non-positive.
func (c *Config) SendConcurrency() int {
	if c.Send.Concurrency > 0 {
		return c.Send.Concurrency
	}
	return DefaultSendConcurrency
}

// Validate checks that configuration required for the service to run is present.
// It is called at boot so misconfiguration surfaces immediately rather than on
// the first user action. Media attachments are always supported, so a
// file-service URL is mandatory: without it, every attachment send would fail.
func (c *Config) Validate() error {
	if c.FileService.URL == "" {
		return fmt.Errorf("FILE_SERVICE_URL is required (media attachment sends fetch bytes from file-service)")
	}
	return nil
}

// Load reads the configuration from config.yaml and overrides it with environment variables.
func Load() (*Config, error) {
	cfg := &Config{}

	// Defaults
	cfg.App.Environment = "development"
	cfg.App.LogLevel = "info"
	cfg.Matrix.BotActorID = "00000000-0000-0000-0000-000000000000"
	cfg.Matrix.BotDisplayName = "Alkemio"

	// Load from file if exists
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	if _, err := os.Stat(configPath); err == nil { //nolint:gosec // config path from env var
		//nolint:gosec // Config file path is controlled by environment variable
		f, err := os.Open(configPath)
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = f.Close()
		}()

		decoder := yaml.NewDecoder(f) //nolint:gosec // Config file path is operator-controlled
		if err := decoder.Decode(cfg); err != nil {
			return nil, err
		}
	}

	loadEnvVars(cfg)

	return cfg, nil
}

func loadEnvVars(cfg *Config) {
	// Override with Env Vars (Consistent with TS Service)
	if v := os.Getenv("ENVIRONMENT"); v != "" {
		cfg.App.Environment = v
	}
	if v := os.Getenv("LOGGING_LEVEL_CONSOLE"); v != "" {
		cfg.App.LogLevel = v
	}

	loadMatrixEnv(cfg)
	loadRabbitMQEnv(cfg)
	loadFileServiceEnv(cfg)
	loadSendEnv(cfg)
}

func loadSendEnv(cfg *Config) {
	if v := os.Getenv("SEND_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Send.TimeoutSeconds = n
		}
	}
	if v := os.Getenv("SEND_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Send.Concurrency = n
		}
	}
}

func loadFileServiceEnv(cfg *Config) {
	if v := os.Getenv("FILE_SERVICE_URL"); v != "" {
		cfg.FileService.URL = v
	}
	if v := os.Getenv("FILE_SERVICE_MAX_ATTACHMENT_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.FileService.MaxAttachmentBytes = n
		}
	}
}

func loadMatrixEnv(cfg *Config) {
	if v := os.Getenv("SYNAPSE_SERVER_URL"); v != "" {
		cfg.Matrix.HomeserverURL = v
	}
	if v := os.Getenv("SYNAPSE_HOMESERVER_NAME"); v != "" {
		cfg.Matrix.HomeserverName = v
	}

	// AppService specific (No direct TS equivalent found in config, keeping standard names)
	if v := os.Getenv("MATRIX_AS_TOKEN"); v != "" {
		cfg.Matrix.AppServiceToken = v
	}
	if v := os.Getenv("MATRIX_HS_TOKEN"); v != "" {
		cfg.Matrix.HomeserverToken = v
	}
	if v := os.Getenv("MATRIX_BOT_ACTOR_ID"); v != "" {
		cfg.Matrix.BotActorID = v
	}
	if v := os.Getenv("MATRIX_BOT_DISPLAY_NAME"); v != "" {
		cfg.Matrix.BotDisplayName = v
	}
	// Both env vars set RegistrationSecret; if both defined, SYNAPSE_REGISTRATION_SECRET wins.
	if v := os.Getenv("SYNAPSE_SERVER_SHARED_SECRET"); v != "" {
		cfg.Matrix.RegistrationSecret = v
	}
	if v := os.Getenv("SYNAPSE_REGISTRATION_SECRET"); v != "" {
		cfg.Matrix.RegistrationSecret = v
	}
}

func loadRabbitMQEnv(cfg *Config) {
	// Support full URL or individual components
	if v := os.Getenv("RABBITMQ_URL"); v != "" {
		cfg.RabbitMQ.URL = v
	} else {
		host := os.Getenv("RABBITMQ_HOST")
		port := os.Getenv("RABBITMQ_PORT")
		user := os.Getenv("RABBITMQ_USER")
		pass := os.Getenv("RABBITMQ_PASSWORD")

		// Default port if not specified
		if port == "" {
			port = "5672"
		}

		if host != "" {
			if user != "" && pass != "" {
				cfg.RabbitMQ.URL = fmt.Sprintf("amqp://%s:%s@%s:%s/", user, pass, host, port)
			} else {
				cfg.RabbitMQ.URL = fmt.Sprintf("amqp://%s:%s/", host, port)
			}
		}
	}
}
