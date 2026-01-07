// Package config handles configuration loading from files and environment variables.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config holds the application configuration.
type Config struct {
	App struct {
		Environment string `yaml:"environment"`
		LogLevel    string `yaml:"log_level"`
	} `yaml:"app"`

	Matrix struct {
		HomeserverURL   string `yaml:"homeserver_url"`
		HomeserverName  string `yaml:"homeserver_name"`
		AppServiceToken string `yaml:"as_token"`
		HomeserverToken string `yaml:"hs_token"`
		BotActorID      string `yaml:"bot_actor_id"` // Bot's Alkemio UUID, used as Matrix localpart
	} `yaml:"matrix"`

	RabbitMQ struct {
		URL string `yaml:"url"`
	} `yaml:"rabbitmq"`

	ActorResolver struct {
		Enabled  bool   `yaml:"enabled"`
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		Database string `yaml:"database"`
	} `yaml:"actor_resolver"`
}

// Load reads the configuration from config.yaml and overrides it with environment variables.
func Load() (*Config, error) {
	cfg := &Config{}

	// Defaults
	cfg.App.Environment = "development"
	cfg.App.LogLevel = "info"
	cfg.Matrix.BotActorID = "00000000-0000-0000-0000-000000000000"

	// ActorResolver defaults (disabled by default)
	cfg.ActorResolver.Enabled = false
	cfg.ActorResolver.Host = "postgres"
	cfg.ActorResolver.Port = "5432"
	cfg.ActorResolver.Username = "synapse"
	cfg.ActorResolver.Password = "synapse"
	cfg.ActorResolver.Database = "alkemio"

	// Load from file if exists
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	if _, err := os.Stat(configPath); err == nil {
		//nolint:gosec // Config file path is controlled by environment variable
		f, err := os.Open(configPath)
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = f.Close()
		}()

		decoder := yaml.NewDecoder(f)
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
	loadActorResolverEnv(cfg)
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

func loadActorResolverEnv(cfg *Config) {
	// Master toggle - ACTOR_ID_MAPPER_ENABLED
	if v := os.Getenv("ACTOR_ID_MAPPER_ENABLED"); v != "" {
		cfg.ActorResolver.Enabled = v == "true" || v == "1"
	}

	// Database connection settings
	if v := os.Getenv("DATABASE_HOST"); v != "" {
		cfg.ActorResolver.Host = v
	}
	if v := os.Getenv("DATABASE_PORT"); v != "" {
		cfg.ActorResolver.Port = v
	}
	if v := os.Getenv("DATABASE_USERNAME"); v != "" {
		cfg.ActorResolver.Username = v
	}
	if v := os.Getenv("DATABASE_PASSWORD"); v != "" {
		cfg.ActorResolver.Password = v
	}
	if v := os.Getenv("DATABASE_NAME"); v != "" {
		cfg.ActorResolver.Database = v
	}
}
