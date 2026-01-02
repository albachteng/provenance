package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Session.Strategy != "smart-time" {
		t.Errorf("expected strategy 'smart-time', got %q", cfg.Session.Strategy)
	}

	if cfg.Session.SmartTime.BaseTimeout.Duration != 30*time.Minute {
		t.Errorf("expected base timeout 30m, got %v", cfg.Session.SmartTime.BaseTimeout.Duration)
	}
	if cfg.Session.SmartTime.ActivityCheckInterval.Duration != 5*time.Minute {
		t.Errorf("expected activity check 5m, got %v", cfg.Session.SmartTime.ActivityCheckInterval.Duration)
	}
	if !cfg.Session.SmartTime.ExtendIfActive {
		t.Error("expected extend_if_active to be true")
	}

	if !cfg.Session.GitEvent.EndOnCommit {
		t.Error("expected end_on_commit to be true")
	}
	if !cfg.Session.GitEvent.EndOnBranchSwitch {
		t.Error("expected end_on_branch_switch to be true")
	}
	if cfg.Session.GitEvent.FallbackTimeout.Duration != 4*time.Hour {
		t.Errorf("expected fallback timeout 4h, got %v", cfg.Session.GitEvent.FallbackTimeout.Duration)
	}

	if cfg.Storage.DBPath != "db.sqlite" {
		t.Errorf("expected db_path 'db.sqlite', got %q", cfg.Storage.DBPath)
	}
	if cfg.Storage.DBPermissions != 0600 {
		t.Errorf("expected db_permissions 0600, got %o", cfg.Storage.DBPermissions)
	}
	if !cfg.Storage.WALMode {
		t.Error("expected wal_mode to be true")
	}

	if cfg.Daemon.SocketPath != "daemon.sock" {
		t.Errorf("expected socket_path 'daemon.sock', got %q", cfg.Daemon.SocketPath)
	}
	if cfg.Daemon.PIDFile != "daemon.pid" {
		t.Errorf("expected pid_file 'daemon.pid', got %q", cfg.Daemon.PIDFile)
	}
	if cfg.Daemon.StartupTimeout.Duration != 5*time.Second {
		t.Errorf("expected startup timeout 5s, got %v", cfg.Daemon.StartupTimeout.Duration)
	}
	if cfg.Daemon.ShutdownTimeout.Duration != 2*time.Second {
		t.Errorf("expected shutdown timeout 2s, got %v", cfg.Daemon.ShutdownTimeout.Duration)
	}
	if cfg.Daemon.SessionCheckInterval.Duration != 1*time.Minute {
		t.Errorf("expected session check interval 1m, got %v", cfg.Daemon.SessionCheckInterval.Duration)
	}

	if cfg.Hooks.HooksDir != "hooks" {
		t.Errorf("expected hooks_dir 'hooks', got %q", cfg.Hooks.HooksDir)
	}

	if !cfg.Redaction.Enabled {
		t.Error("expected redaction to be enabled")
	}
	if len(cfg.Redaction.BuiltinPatterns) == 0 {
		t.Error("expected builtin patterns to be populated")
	}
}

func TestDurationUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "minutes",
			yaml:     "timeout: 30m",
			expected: 30 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "hours",
			yaml:     "timeout: 2h",
			expected: 2 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "seconds",
			yaml:     "timeout: 45s",
			expected: 45 * time.Second,
			wantErr:  false,
		},
		{
			name:     "composite",
			yaml:     "timeout: 1h30m",
			expected: 90 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "invalid format",
			yaml:     "timeout: invalid",
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result struct {
				Timeout Duration `yaml:"timeout"`
			}

			err := yaml.Unmarshal([]byte(tt.yaml), &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result.Timeout.Duration != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result.Timeout.Duration)
			}
		})
	}
}

func TestDurationMarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "30 minutes",
			duration: 30 * time.Minute,
			expected: "timeout: 30m0s\n",
		},
		{
			name:     "2 hours",
			duration: 2 * time.Hour,
			expected: "timeout: 2h0m0s\n",
		},
		{
			name:     "45 seconds",
			duration: 45 * time.Second,
			expected: "timeout: 45s\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := struct {
				Timeout Duration `yaml:"timeout"`
			}{
				Timeout: Duration{tt.duration},
			}

			data, err := yaml.Marshal(input)
			if err != nil {
				t.Fatalf("MarshalYAML() error = %v", err)
			}

			if string(data) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(data))
			}
		})
	}
}

func TestResolvePaths(t *testing.T) {
	tests := []struct {
		name     string
		baseDir  string
		path     string
		expected string
	}{
		{
			name:     "relative path",
			baseDir:  "/home/user/.ai-provenance",
			path:     "db.sqlite",
			expected: "/home/user/.ai-provenance/db.sqlite",
		},
		{
			name:     "absolute path unchanged",
			baseDir:  "/home/user/.ai-provenance",
			path:     "/var/lib/provenance/db.sqlite",
			expected: "/var/lib/provenance/db.sqlite",
		},
		{
			name:     "relative subdirectory",
			baseDir:  "/home/user/.ai-provenance",
			path:     "data/events.db",
			expected: "/home/user/.ai-provenance/data/events.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Storage.DBPath = tt.path
			cfg.Daemon.SocketPath = tt.path
			cfg.Daemon.PIDFile = tt.path
			cfg.Hooks.HooksDir = tt.path

			err := cfg.ResolvePaths(tt.baseDir)
			if err != nil {
				t.Fatalf("ResolvePaths() error = %v", err)
			}

			if cfg.Storage.DBPath != tt.expected {
				t.Errorf("DBPath: expected %q, got %q", tt.expected, cfg.Storage.DBPath)
			}
			if cfg.Daemon.SocketPath != tt.expected {
				t.Errorf("SocketPath: expected %q, got %q", tt.expected, cfg.Daemon.SocketPath)
			}
			if cfg.Daemon.PIDFile != tt.expected {
				t.Errorf("PIDFile: expected %q, got %q", tt.expected, cfg.Daemon.PIDFile)
			}
			if cfg.Hooks.HooksDir != tt.expected {
				t.Errorf("HooksDir: expected %q, got %q", tt.expected, cfg.Hooks.HooksDir)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name: "invalid strategy",
			modify: func(c *Config) {
				c.Session.Strategy = "invalid"
			},
			wantErr: true,
			errMsg:  "invalid session strategy",
		},
		{
			name: "negative base timeout",
			modify: func(c *Config) {
				c.Session.SmartTime.BaseTimeout = Duration{-1 * time.Minute}
			},
			wantErr: true,
			errMsg:  "base_timeout must be positive",
		},
		{
			name: "zero activity check interval",
			modify: func(c *Config) {
				c.Session.SmartTime.ActivityCheckInterval = Duration{0}
			},
			wantErr: true,
			errMsg:  "activity_check_interval must be positive",
		},
		{
			name: "negative fallback timeout",
			modify: func(c *Config) {
				c.Session.GitEvent.FallbackTimeout = Duration{-1 * time.Hour}
			},
			wantErr: true,
			errMsg:  "fallback_timeout must be positive",
		},
		{
			name: "negative startup timeout",
			modify: func(c *Config) {
				c.Daemon.StartupTimeout = Duration{-1 * time.Second}
			},
			wantErr: true,
			errMsg:  "startup_timeout must be positive",
		},
		{
			name: "negative shutdown timeout",
			modify: func(c *Config) {
				c.Daemon.ShutdownTimeout = Duration{-1 * time.Second}
			},
			wantErr: true,
			errMsg:  "shutdown_timeout must be positive",
		},
		{
			name: "negative session check interval",
			modify: func(c *Config) {
				c.Daemon.SessionCheckInterval = Duration{-1 * time.Minute}
			},
			wantErr: true,
			errMsg:  "session_check_interval must be positive",
		},
		{
			name: "empty db path",
			modify: func(c *Config) {
				c.Storage.DBPath = ""
			},
			wantErr: true,
			errMsg:  "db_path cannot be empty",
		},
		{
			name: "empty socket path",
			modify: func(c *Config) {
				c.Daemon.SocketPath = ""
			},
			wantErr: true,
			errMsg:  "socket_path cannot be empty",
		},
		{
			name: "empty pid file",
			modify: func(c *Config) {
				c.Daemon.PIDFile = ""
			},
			wantErr: true,
			errMsg:  "pid_file cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.modify(cfg)

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			}
		})
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
