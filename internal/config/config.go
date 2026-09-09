// Package config handles configuration loading from files and environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// defaultSetChildrenTimeoutSeconds is the execution deadline one
// communication.hierarchy.set_children call gives itself when
// hierarchy.set_children_timeout_seconds is not configured. Sized to leave
// headroom under the server's 10s RPC timeout.
const defaultSetChildrenTimeoutSeconds = 8

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

	Hierarchy struct {
		// StateEventsPerSecond paces m.space.child add/remove/prune writes issued by
		// one hierarchy convergence call. Kept below Synapse's rc_message rate so a
		// pass is never itself the cause of a rate-limit rejection.
		StateEventsPerSecond float64 `yaml:"state_events_per_second"`
		// ParentPointerEventsPerSecond paces the separate, opt-in room-side
		// m.space.parent repair, which can force an admin-join and so draws from
		// Synapse's tighter join rate limit instead.
		ParentPointerEventsPerSecond float64 `yaml:"parent_pointer_events_per_second"`
		// ParentPointerBudgetPerCall caps how many parent-pointer repairs one
		// convergence call will attempt; touched children beyond the cap are
		// reported as deferred rather than paced indefinitely. Not measured against
		// live data yet — see the plan's pre-freeze measurement task.
		ParentPointerBudgetPerCall int `yaml:"parent_pointer_budget_per_call"`
		// MaxWriteOperationsPerCall hard-caps the total number of state-event
		// writes (adds, removals, prunes, and parent-pointer repairs combined)
		// one convergence call will attempt, independent of how long that would
		// otherwise take under the pacing budgets above. Zero means unbounded.
		// This is the call's own internal stop condition — it does not depend on
		// the caller enforcing a deadline, since the queue transport this
		// operation is served over carries no cancellation signal of its own.
		MaxWriteOperationsPerCall int `yaml:"max_write_operations_per_call"`
		// SetChildrenTimeoutSeconds bounds one set_children call's own
		// execution, independent of whether the caller is still waiting. The
		// RabbitMQ/Watermill transport this operation is served over carries no
		// cancellation signal a caller-side RPC timeout can propagate back to
		// the running handler, so the handler imposes this deadline on itself.
		// It is sized to leave headroom under the server's RPC timeout for the
		// response to marshal and travel back before that RPC call gives up —
		// raise this alongside a raised server RPC timeout (the documented A-10
		// fallback for a category that outgrows the RPC envelope even after
		// maxOperations sizing; see contracts/hierarchy-set-children.md).
		SetChildrenTimeoutSeconds float64 `yaml:"set_children_timeout_seconds"`
	} `yaml:"hierarchy"`
}

// Load reads the configuration from config.yaml and overrides it with environment variables.
func Load() (*Config, error) {
	cfg := &Config{}

	// Defaults
	cfg.App.Environment = "development"
	cfg.App.LogLevel = "info"
	cfg.Matrix.BotActorID = "00000000-0000-0000-0000-000000000000"
	cfg.Matrix.BotDisplayName = "Alkemio"
	cfg.Hierarchy.StateEventsPerSecond = 5
	cfg.Hierarchy.ParentPointerEventsPerSecond = 1
	cfg.Hierarchy.ParentPointerBudgetPerCall = 5
	cfg.Hierarchy.MaxWriteOperationsPerCall = 500
	cfg.Hierarchy.SetChildrenTimeoutSeconds = defaultSetChildrenTimeoutSeconds

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

	if err := validateHierarchy(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validateHierarchy rejects hierarchy settings that would silently disable the
// operation they configure rather than merely tune it. An omitted
// set_children_timeout_seconds keeps its default, but an explicitly configured
// non-positive one — from YAML or HIERARCHY_SET_CHILDREN_TIMEOUT_SECONDS —
// makes context.WithTimeout hand every set_children call an already-expired
// deadline, so the handler aborts before its first write and reports
// DEADLINE_EXCEEDED for the rest of the process's life. That is a startup
// misconfiguration and is reported as one here, loudly, rather than absorbed
// at call time where it is indistinguishable from a genuinely oversized
// convergence. The pacing rates need no equivalent guard:
// newStateWriteRateLimiter already floors a non-positive rate at a
// conservative 1 event/s, and a non-positive max_write_operations_per_call is
// documented as "unbounded" on purpose.
func validateHierarchy(cfg *Config) error {
	if cfg.Hierarchy.SetChildrenTimeoutSeconds <= 0 {
		return fmt.Errorf(
			"invalid hierarchy.set_children_timeout_seconds %v: must be greater than 0 (omit it for the %ds default)",
			cfg.Hierarchy.SetChildrenTimeoutSeconds, defaultSetChildrenTimeoutSeconds)
	}
	return nil
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
	loadHierarchyEnv(cfg)
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

func loadHierarchyEnv(cfg *Config) {
	if v := os.Getenv("HIERARCHY_STATE_EVENTS_PER_SECOND"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Hierarchy.StateEventsPerSecond = parsed
		}
	}
	if v := os.Getenv("HIERARCHY_PARENT_POINTER_EVENTS_PER_SECOND"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Hierarchy.ParentPointerEventsPerSecond = parsed
		}
	}
	if v := os.Getenv("HIERARCHY_PARENT_POINTER_BUDGET_PER_CALL"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Hierarchy.ParentPointerBudgetPerCall = parsed
		}
	}
	if v := os.Getenv("HIERARCHY_MAX_WRITE_OPERATIONS_PER_CALL"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Hierarchy.MaxWriteOperationsPerCall = parsed
		}
	}
	if v := os.Getenv("HIERARCHY_SET_CHILDREN_TIMEOUT_SECONDS"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Hierarchy.SetChildrenTimeoutSeconds = parsed
		}
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
