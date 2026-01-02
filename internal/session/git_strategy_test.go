package session

import (
	"testing"
	"time"
)

// TestGitEventStrategyName tests that the strategy returns correct name
func TestGitEventStrategyName(t *testing.T) {
	strategy := NewGitEventStrategy(true, true, 4*time.Hour)

	if strategy.Name() != "git-event" {
		t.Errorf("Expected strategy name 'git-event', got: %s", strategy.Name())
	}
}

// TestGitEventStrategyShouldStartSession tests session start conditions
func TestGitEventStrategyShouldStartSession(t *testing.T) {
	strategy := NewGitEventStrategy(true, true, 4*time.Hour)

	tests := []struct {
		name     string
		ctx      *Context
		expected bool
	}{
		{
			name: "no active session",
			ctx: &Context{
				SessionID: "",
			},
			expected: true,
		},
		{
			name: "active session exists",
			ctx: &Context{
				SessionID:    "session-123",
				SessionStart: time.Now().Add(-1 * time.Hour),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.ShouldStartSession(tt.ctx)
			if result != tt.expected {
				t.Errorf("ShouldStartSession() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGitEventStrategyShouldEndOnCommit tests ending session on commit
func TestGitEventStrategyShouldEndOnCommit(t *testing.T) {
	strategy := NewGitEventStrategy(true, false, 4*time.Hour) // endOnCommit=true, endOnBranchSwitch=false

	now := time.Now()

	tests := []struct {
		name     string
		ctx      *Context
		expected bool
	}{
		{
			name: "commit detected - should end",
			ctx: &Context{
				SessionID:     "session-123",
				SessionStart:  now.Add(-30 * time.Minute),
				GitCommit:     "abc123",
				LastGitCommit: "def456", // Different commit
			},
			expected: true,
		},
		{
			name: "no commit change - should not end",
			ctx: &Context{
				SessionID:     "session-123",
				SessionStart:  now.Add(-30 * time.Minute),
				GitCommit:     "abc123",
				LastGitCommit: "abc123", // Same commit
			},
			expected: false,
		},
		{
			name: "first check (no last commit) - should not end",
			ctx: &Context{
				SessionID:     "session-123",
				SessionStart:  now.Add(-30 * time.Minute),
				GitCommit:     "abc123",
				LastGitCommit: "", // No previous commit tracked
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.ShouldEndSession(tt.ctx)
			if result != tt.expected {
				t.Errorf("ShouldEndSession() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGitEventStrategyShouldEndOnBranchSwitch tests ending session on branch switch
func TestGitEventStrategyShouldEndOnBranchSwitch(t *testing.T) {
	strategy := NewGitEventStrategy(false, true, 4*time.Hour) // endOnCommit=false, endOnBranchSwitch=true

	now := time.Now()

	tests := []struct {
		name     string
		ctx      *Context
		expected bool
	}{
		{
			name: "branch switched - should end",
			ctx: &Context{
				SessionID:     "session-123",
				SessionStart:  now.Add(-30 * time.Minute),
				GitBranch:     "feature-new",
				LastGitBranch: "main",
			},
			expected: true,
		},
		{
			name: "same branch - should not end",
			ctx: &Context{
				SessionID:     "session-123",
				SessionStart:  now.Add(-30 * time.Minute),
				GitBranch:     "main",
				LastGitBranch: "main",
			},
			expected: false,
		},
		{
			name: "first check (no last branch) - should not end",
			ctx: &Context{
				SessionID:     "session-123",
				SessionStart:  now.Add(-30 * time.Minute),
				GitBranch:     "main",
				LastGitBranch: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.ShouldEndSession(tt.ctx)
			if result != tt.expected {
				t.Errorf("ShouldEndSession() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGitEventStrategyFallbackTimeout tests the fallback timeout safety valve
func TestGitEventStrategyFallbackTimeout(t *testing.T) {
	fallbackTimeout := 4 * time.Hour
	strategy := NewGitEventStrategy(false, false, fallbackTimeout) // Both git events disabled

	now := time.Now()

	tests := []struct {
		name     string
		ctx      *Context
		expected bool
	}{
		{
			name: "within fallback timeout - should not end",
			ctx: &Context{
				SessionID:      "session-123",
				SessionStart:   now.Add(-2 * time.Hour),
				TimeSinceStart: 2 * time.Hour,
			},
			expected: false,
		},
		{
			name: "exceeded fallback timeout - should end",
			ctx: &Context{
				SessionID:      "session-123",
				SessionStart:   now.Add(-5 * time.Hour),
				TimeSinceStart: 5 * time.Hour,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.ShouldEndSession(tt.ctx)
			if result != tt.expected {
				t.Errorf("ShouldEndSession() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGitEventStrategyCombinedConditions tests commit + branch switch together
func TestGitEventStrategyCombinedConditions(t *testing.T) {
	strategy := NewGitEventStrategy(true, true, 4*time.Hour) // Both enabled

	now := time.Now()

	tests := []struct {
		name     string
		ctx      *Context
		expected bool
	}{
		{
			name: "commit changed - should end (even if branch same)",
			ctx: &Context{
				SessionID:     "session-123",
				SessionStart:  now.Add(-30 * time.Minute),
				GitBranch:     "main",
				LastGitBranch: "main",
				GitCommit:     "abc123",
				LastGitCommit: "def456",
			},
			expected: true,
		},
		{
			name: "branch changed - should end (even if commit same)",
			ctx: &Context{
				SessionID:     "session-123",
				SessionStart:  now.Add(-30 * time.Minute),
				GitBranch:     "feature",
				LastGitBranch: "main",
				GitCommit:     "abc123",
				LastGitCommit: "abc123",
			},
			expected: true,
		},
		{
			name: "both changed - should end",
			ctx: &Context{
				SessionID:     "session-123",
				SessionStart:  now.Add(-30 * time.Minute),
				GitBranch:     "feature",
				LastGitBranch: "main",
				GitCommit:     "abc123",
				LastGitCommit: "def456",
			},
			expected: true,
		},
		{
			name: "neither changed - should not end",
			ctx: &Context{
				SessionID:     "session-123",
				SessionStart:  now.Add(-30 * time.Minute),
				GitBranch:     "main",
				LastGitBranch: "main",
				GitCommit:     "abc123",
				LastGitCommit: "abc123",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.ShouldEndSession(tt.ctx)
			if result != tt.expected {
				t.Errorf("ShouldEndSession() = %v, want %v", result, tt.expected)
			}
		})
	}
}
