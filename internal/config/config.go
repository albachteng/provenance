package config

import (
	"fmt"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete provenance configuration
type Config struct {
	Session   SessionConfig   `yaml:"session"`
	Storage   StorageConfig   `yaml:"storage"`
	Daemon    DaemonConfig    `yaml:"daemon"`
	Hooks     HooksConfig     `yaml:"hooks"`
	Redaction RedactionConfig `yaml:"redaction"`
}

// SessionConfig configures session boundary detection
type SessionConfig struct {
	Strategy  string          `yaml:"strategy"` // "smart-time" or "git-event"
	SmartTime SmartTimeConfig `yaml:"smart_time"`
	GitEvent  GitEventConfig  `yaml:"git_event"`
}

// SmartTimeConfig configures the smart-time session strategy
type SmartTimeConfig struct {
	BaseTimeout           Duration `yaml:"base_timeout"`
	ActivityCheckInterval Duration `yaml:"activity_check_interval"`
	// ExtendIfActive uses *bool to distinguish between explicit false and unset.
	// Without the pointer, YAML unmarshaling can't tell the difference between
	// "extend_if_active: false" (explicit) and omitting the field (unset).
	// This allows config file layering to work correctly.
	ExtendIfActive *bool `yaml:"extend_if_active,omitempty"`
}

// GitEventConfig configures the git-event session strategy
type GitEventConfig struct {
	// EndOnCommit and EndOnBranchSwitch use *bool to distinguish between explicit false and unset.
	// Without pointers, YAML unmarshaling can't tell the difference between
	// "end_on_commit: false" (explicit) and omitting the field (unset).
	// This allows config file layering to work correctly.
	EndOnCommit       *bool    `yaml:"end_on_commit,omitempty"`
	EndOnBranchSwitch *bool    `yaml:"end_on_branch_switch,omitempty"`
	FallbackTimeout   Duration `yaml:"fallback_timeout"`
}

// StorageConfig configures database and storage
type StorageConfig struct {
	DBPath        string `yaml:"db_path"`
	DBPermissions uint32 `yaml:"db_permissions"`
	WALMode       bool   `yaml:"wal_mode"`
}

// DaemonConfig configures daemon behavior
type DaemonConfig struct {
	SocketPath           string   `yaml:"socket_path"`
	PIDFile              string   `yaml:"pid_file"`
	StartupTimeout       Duration `yaml:"startup_timeout"`
	ShutdownTimeout      Duration `yaml:"shutdown_timeout"`
	SessionCheckInterval Duration `yaml:"session_check_interval"`
}

// HooksConfig configures git hooks
type HooksConfig struct {
	HooksDir string `yaml:"hooks_dir"`
}

// RedactionConfig configures sensitive data redaction
type RedactionConfig struct {
	Enabled         bool     `yaml:"enabled"`
	BuiltinPatterns []string `yaml:"builtin_patterns"`
	CustomPatterns  []string `yaml:"custom_patterns"`
}

// Duration wraps time.Duration for YAML parsing
type Duration struct {
	time.Duration
}

// UnmarshalYAML parses a duration string (e.g., "30m", "2h") from YAML
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

// MarshalYAML converts duration to string for YAML output
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.Duration.String(), nil
}

// boolPtr returns a pointer to a bool value
// Helper for creating default config with pointer booleans
func boolPtr(b bool) *bool {
	return &b
}

// Default returns a Config with sensible defaults
// These match current test values for backward compatibility
func Default() *Config {
	return &Config{
		Session: SessionConfig{
			Strategy: "smart-time",
			SmartTime: SmartTimeConfig{
				BaseTimeout:           Duration{30 * time.Minute},
				ActivityCheckInterval: Duration{5 * time.Minute},
				ExtendIfActive:        boolPtr(true),
			},
			GitEvent: GitEventConfig{
				EndOnCommit:       boolPtr(true),
				EndOnBranchSwitch: boolPtr(true),
				FallbackTimeout:   Duration{4 * time.Hour},
			},
		},
		Storage: StorageConfig{
			DBPath:        "db.sqlite",
			DBPermissions: 0600,
			WALMode:       true,
		},
		Daemon: DaemonConfig{
			SocketPath:           "daemon.sock",
			PIDFile:              "daemon.pid",
			StartupTimeout:       Duration{5 * time.Second},
			ShutdownTimeout:      Duration{2 * time.Second},
			SessionCheckInterval: Duration{1 * time.Minute},
		},
		Hooks: HooksConfig{
			HooksDir: "hooks",
		},
		Redaction: RedactionConfig{
			Enabled:         true,
			BuiltinPatterns: []string{"password", "api_key", "secret"},
			CustomPatterns:  []string{},
		},
	}
}

// ResolvePaths converts relative paths to absolute paths based on baseDir
func (c *Config) ResolvePaths(baseDir string) error {
	c.Storage.DBPath = resolvePath(c.Storage.DBPath, baseDir)
	c.Daemon.SocketPath = resolvePath(c.Daemon.SocketPath, baseDir)
	c.Daemon.PIDFile = resolvePath(c.Daemon.PIDFile, baseDir)
	c.Hooks.HooksDir = resolvePath(c.Hooks.HooksDir, baseDir)
	return nil
}

// resolvePath returns an absolute path, resolving relative paths against baseDir
func resolvePath(path, baseDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Session.Strategy != "smart-time" && c.Session.Strategy != "git-event" {
		return fmt.Errorf("invalid session strategy: %q (must be 'smart-time' or 'git-event')", c.Session.Strategy)
	}

	if c.Session.SmartTime.BaseTimeout.Duration <= 0 {
		return fmt.Errorf("session.smart_time.base_timeout must be positive")
	}
	if c.Session.SmartTime.ActivityCheckInterval.Duration <= 0 {
		return fmt.Errorf("session.smart_time.activity_check_interval must be positive")
	}

	if c.Session.GitEvent.FallbackTimeout.Duration <= 0 {
		return fmt.Errorf("session.git_event.fallback_timeout must be positive")
	}

	if c.Daemon.StartupTimeout.Duration <= 0 {
		return fmt.Errorf("daemon.startup_timeout must be positive")
	}
	if c.Daemon.ShutdownTimeout.Duration <= 0 {
		return fmt.Errorf("daemon.shutdown_timeout must be positive")
	}
	if c.Daemon.SessionCheckInterval.Duration <= 0 {
		return fmt.Errorf("daemon.session_check_interval must be positive")
	}

	if c.Storage.DBPath == "" {
		return fmt.Errorf("storage.db_path cannot be empty")
	}
	if c.Daemon.SocketPath == "" {
		return fmt.Errorf("daemon.socket_path cannot be empty")
	}
	if c.Daemon.PIDFile == "" {
		return fmt.Errorf("daemon.pid_file cannot be empty")
	}

	return nil
}
