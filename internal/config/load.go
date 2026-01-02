package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Load loads configuration from all sources and merges them
// Priority: env vars > per-repo config > global config > defaults
func Load(provenanceHome string, repoPath string) (*Config, error) {
	cfg := Default()

	globalPath := filepath.Join(provenanceHome, "config.yaml")
	if err := loadFile(globalPath, cfg); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}

	if repoPath != "" {
		repoConfigPath := filepath.Join(repoPath, ".ai-provenance", "config.yaml")
		if err := loadFile(repoConfigPath, cfg); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load repo config: %w", err)
		}
	}

	applyEnvOverrides(cfg)

	if err := cfg.ResolvePaths(provenanceHome); err != nil {
		return nil, fmt.Errorf("failed to resolve paths: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// loadFile loads a YAML config file and merges it into cfg
func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	mergeConfig(cfg, &fileCfg)

	return nil
}

// applyEnvOverrides applies environment variable overrides
func applyEnvOverrides(cfg *Config) {
	if strategy := os.Getenv("AI_PROVENANCE_SESSION_STRATEGY"); strategy != "" {
		cfg.Session.Strategy = strategy
	}

	if timeout := os.Getenv("AI_PROVENANCE_SESSION_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			cfg.Session.SmartTime.BaseTimeout = Duration{d}
		}
	}

	if dbPath := os.Getenv("AI_PROVENANCE_DB_PATH"); dbPath != "" {
		cfg.Storage.DBPath = dbPath
	}
}

// mergeConfig merges src into dst (non-zero values only)
func mergeConfig(dst, src *Config) {
	if src.Session.Strategy != "" {
		dst.Session.Strategy = src.Session.Strategy
	}

	if src.Session.SmartTime.BaseTimeout.Duration != 0 {
		dst.Session.SmartTime.BaseTimeout = src.Session.SmartTime.BaseTimeout
	}
	if src.Session.SmartTime.ActivityCheckInterval.Duration != 0 {
		dst.Session.SmartTime.ActivityCheckInterval = src.Session.SmartTime.ActivityCheckInterval
	}
	// Note: ExtendIfActive boolean can't be overridden to false via config
	// due to inability to distinguish explicit false from zero value

	// Git event settings
	if src.Session.GitEvent.FallbackTimeout.Duration != 0 {
		dst.Session.GitEvent.FallbackTimeout = src.Session.GitEvent.FallbackTimeout
	}
	// Note: EndOnCommit and EndOnBranchSwitch booleans can't be overridden to false
	// due to inability to distinguish explicit false from zero value

	if src.Storage.DBPath != "" {
		dst.Storage.DBPath = src.Storage.DBPath
	}
	if src.Storage.DBPermissions != 0 {
		dst.Storage.DBPermissions = src.Storage.DBPermissions
	}

	if src.Daemon.SocketPath != "" {
		dst.Daemon.SocketPath = src.Daemon.SocketPath
	}
	if src.Daemon.PIDFile != "" {
		dst.Daemon.PIDFile = src.Daemon.PIDFile
	}
	if src.Daemon.StartupTimeout.Duration != 0 {
		dst.Daemon.StartupTimeout = src.Daemon.StartupTimeout
	}
	if src.Daemon.ShutdownTimeout.Duration != 0 {
		dst.Daemon.ShutdownTimeout = src.Daemon.ShutdownTimeout
	}
	if src.Daemon.SessionCheckInterval.Duration != 0 {
		dst.Daemon.SessionCheckInterval = src.Daemon.SessionCheckInterval
	}

	if src.Hooks.HooksDir != "" {
		dst.Hooks.HooksDir = src.Hooks.HooksDir
	}

	if len(src.Redaction.BuiltinPatterns) > 0 {
		dst.Redaction.BuiltinPatterns = src.Redaction.BuiltinPatterns
	}
	if len(src.Redaction.CustomPatterns) > 0 {
		dst.Redaction.CustomPatterns = src.Redaction.CustomPatterns
	}
}
