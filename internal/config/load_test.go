package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_DefaultsOnly(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := Load(tmpDir, "")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Session.Strategy != "smart-time" {
		t.Errorf("expected strategy 'smart-time', got %q", cfg.Session.Strategy)
	}
	if cfg.Storage.DBPath != filepath.Join(tmpDir, "db.sqlite") {
		t.Errorf("expected db path %q, got %q", filepath.Join(tmpDir, "db.sqlite"), cfg.Storage.DBPath)
	}
}

func TestLoad_GlobalConfig(t *testing.T) {
	tmpDir := t.TempDir()

	globalConfig := `
session:
  strategy: "git-event"
  git_event:
    fallback_timeout: "2h"
daemon:
  session_check_interval: "30s"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(globalConfig), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(tmpDir, "")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Session.Strategy != "git-event" {
		t.Errorf("expected strategy 'git-event', got %q", cfg.Session.Strategy)
	}
	if cfg.Session.GitEvent.FallbackTimeout.Duration != 2*time.Hour {
		t.Errorf("expected fallback timeout 2h, got %v", cfg.Session.GitEvent.FallbackTimeout.Duration)
	}
	// Boolean fields keep default values if not explicitly set
	if cfg.Session.GitEvent.EndOnCommit == nil || !*cfg.Session.GitEvent.EndOnCommit {
		t.Error("expected end_on_commit to remain true (default)")
	}
	if cfg.Session.GitEvent.EndOnBranchSwitch == nil || !*cfg.Session.GitEvent.EndOnBranchSwitch {
		t.Error("expected end_on_branch_switch to remain true (default)")
	}
	if cfg.Daemon.SessionCheckInterval.Duration != 30*time.Second {
		t.Errorf("expected session check interval 30s, got %v", cfg.Daemon.SessionCheckInterval.Duration)
	}

	if cfg.Session.SmartTime.BaseTimeout.Duration != 30*time.Minute {
		t.Errorf("expected base timeout to remain 30m, got %v", cfg.Session.SmartTime.BaseTimeout.Duration)
	}
}

func TestLoad_RepoConfigOverridesGlobal(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".ai-provenance"), 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	globalConfig := `
session:
  strategy: "smart-time"
  smart_time:
    base_timeout: "45m"
daemon:
  session_check_interval: "2m"
`
	globalPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(globalPath, []byte(globalConfig), 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	repoConfig := `
session:
  strategy: "git-event"
daemon:
  session_check_interval: "30s"
`
	repoPath := filepath.Join(repoDir, ".ai-provenance", "config.yaml")
	if err := os.WriteFile(repoPath, []byte(repoConfig), 0644); err != nil {
		t.Fatalf("failed to write repo config: %v", err)
	}

	cfg, err := Load(tmpDir, repoDir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Repo config should override global
	if cfg.Session.Strategy != "git-event" {
		t.Errorf("expected strategy 'git-event' from repo config, got %q", cfg.Session.Strategy)
	}
	if cfg.Daemon.SessionCheckInterval.Duration != 30*time.Second {
		t.Errorf("expected session check interval 30s from repo config, got %v", cfg.Daemon.SessionCheckInterval.Duration)
	}

	if cfg.Session.SmartTime.BaseTimeout.Duration != 45*time.Minute {
		t.Errorf("expected base timeout 45m from global config, got %v", cfg.Session.SmartTime.BaseTimeout.Duration)
	}
}

func TestLoad_EnvironmentOverrides(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("AI_PROVENANCE_SESSION_STRATEGY", "git-event")
	t.Setenv("AI_PROVENANCE_SESSION_TIMEOUT", "1h")
	t.Setenv("AI_PROVENANCE_DB_PATH", "/custom/db.sqlite")

	cfg, err := Load(tmpDir, "")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Session.Strategy != "git-event" {
		t.Errorf("expected strategy 'git-event' from env, got %q", cfg.Session.Strategy)
	}
	if cfg.Session.SmartTime.BaseTimeout.Duration != 1*time.Hour {
		t.Errorf("expected base timeout 1h from env, got %v", cfg.Session.SmartTime.BaseTimeout.Duration)
	}
	if cfg.Storage.DBPath != "/custom/db.sqlite" {
		t.Errorf("expected db path '/custom/db.sqlite' from env, got %q", cfg.Storage.DBPath)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	invalidYAML := `
session:
  strategy: "smart-time"
  invalid yaml here: [
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(tmpDir, "")
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestLoad_InvalidStrategy(t *testing.T) {
	tmpDir := t.TempDir()

	invalidConfig := `
session:
  strategy: "invalid-strategy"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(tmpDir, "")
	if err == nil {
		t.Error("expected validation error for invalid strategy, got nil")
	}
	if err != nil && !contains(err.Error(), "invalid session strategy") {
		t.Errorf("expected error about invalid strategy, got: %v", err)
	}
}

func TestLoad_MissingFiles(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := Load(tmpDir, "")
	if err != nil {
		t.Fatalf("Load() should succeed with missing files: %v", err)
	}

	if cfg.Session.Strategy != "smart-time" {
		t.Errorf("expected default strategy 'smart-time', got %q", cfg.Session.Strategy)
	}
}

func TestLoad_PathResolution(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := Load(tmpDir, "")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	expectedDBPath := filepath.Join(tmpDir, "db.sqlite")
	if cfg.Storage.DBPath != expectedDBPath {
		t.Errorf("expected db path %q, got %q", expectedDBPath, cfg.Storage.DBPath)
	}

	expectedSocketPath := filepath.Join(tmpDir, "daemon.sock")
	if cfg.Daemon.SocketPath != expectedSocketPath {
		t.Errorf("expected socket path %q, got %q", expectedSocketPath, cfg.Daemon.SocketPath)
	}

	expectedPIDFile := filepath.Join(tmpDir, "daemon.pid")
	if cfg.Daemon.PIDFile != expectedPIDFile {
		t.Errorf("expected pid file %q, got %q", expectedPIDFile, cfg.Daemon.PIDFile)
	}

	expectedHooksDir := filepath.Join(tmpDir, "hooks")
	if cfg.Hooks.HooksDir != expectedHooksDir {
		t.Errorf("expected hooks dir %q, got %q", expectedHooksDir, cfg.Hooks.HooksDir)
	}
}

func TestLoad_NegativeDurations(t *testing.T) {
	tmpDir := t.TempDir()

	negativeConfig := `
session:
  smart_time:
    base_timeout: "-30m"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(negativeConfig), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(tmpDir, "")
	if err == nil {
		t.Error("expected validation error for negative duration, got nil")
	}
}

func TestLoad_ExplicitFalseBooleans(t *testing.T) {
	tmpDir := t.TempDir()

	// Test that we can explicitly set booleans to false (pointer booleans)
	configWithFalse := `
session:
  strategy: "git-event"
  git_event:
    end_on_commit: false
    end_on_branch_switch: false
    fallback_timeout: "2h"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configWithFalse), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(tmpDir, "")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify booleans were explicitly set to false
	if cfg.Session.GitEvent.EndOnCommit == nil {
		t.Error("expected end_on_commit to be set (non-nil)")
	} else if *cfg.Session.GitEvent.EndOnCommit {
		t.Error("expected end_on_commit to be false")
	}

	if cfg.Session.GitEvent.EndOnBranchSwitch == nil {
		t.Error("expected end_on_branch_switch to be set (non-nil)")
	} else if *cfg.Session.GitEvent.EndOnBranchSwitch {
		t.Error("expected end_on_branch_switch to be false")
	}
}
