package session

import (
	"testing"
	"time"
)

// TestSmartTimeStrategyName tests that the strategy returns correct name
func TestSmartTimeStrategyName(t *testing.T) {
	strategy := NewSmartTimeStrategy(30*time.Minute, 5*time.Minute, true)

	if strategy.Name() != "smart-time" {
		t.Errorf("Expected strategy name 'smart-time', got: %s", strategy.Name())
	}
}

// TestSmartTimeStrategyShouldStartSession tests session start conditions
func TestSmartTimeStrategyShouldStartSession(t *testing.T) {
	strategy := NewSmartTimeStrategy(30*time.Minute, 5*time.Minute, true)

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
				SessionStart: time.Now().Add(-10 * time.Minute),
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

// TestSmartTimeStrategyShouldEndSession tests session end conditions
func TestSmartTimeStrategyShouldEndSession(t *testing.T) {
	baseTimeout := 30 * time.Minute
	strategy := NewSmartTimeStrategy(baseTimeout, 5*time.Minute, true)

	now := time.Now()

	tests := []struct {
		name     string
		ctx      *Context
		expected bool
	}{
		{
			name: "recent activity, LLM active - should not end",
			ctx: &Context{
				SessionID:          "session-123",
				SessionStart:       now.Add(-10 * time.Minute),
				LastEventTime:      now.Add(-2 * time.Minute),
				IsLLMActive:        true,
				TimeSinceLastEvent: 2 * time.Minute,
				TimeSinceStart:     10 * time.Minute,
			},
			expected: false,
		},
		{
			name: "recent activity, LLM idle - should not end",
			ctx: &Context{
				SessionID:          "session-123",
				SessionStart:       now.Add(-10 * time.Minute),
				LastEventTime:      now.Add(-2 * time.Minute),
				IsLLMActive:        false,
				TimeSinceLastEvent: 2 * time.Minute,
				TimeSinceStart:     10 * time.Minute,
			},
			expected: false,
		},
		{
			name: "timeout exceeded, LLM idle - should end",
			ctx: &Context{
				SessionID:          "session-123",
				SessionStart:       now.Add(-45 * time.Minute),
				LastEventTime:      now.Add(-35 * time.Minute),
				IsLLMActive:        false,
				TimeSinceLastEvent: 35 * time.Minute,
				TimeSinceStart:     45 * time.Minute,
			},
			expected: true,
		},
		{
			name: "timeout exceeded, LLM active - should not end (extend if active)",
			ctx: &Context{
				SessionID:          "session-123",
				SessionStart:       now.Add(-45 * time.Minute),
				LastEventTime:      now.Add(-35 * time.Minute),
				IsLLMActive:        true,
				TimeSinceLastEvent: 35 * time.Minute,
				TimeSinceStart:     45 * time.Minute,
			},
			expected: false,
		},
		{
			name: "no session active - should not end",
			ctx: &Context{
				SessionID: "",
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

// TestSmartTimeStrategyDisableExtendIfActive tests behavior when extend_if_active is false
func TestSmartTimeStrategyDisableExtendIfActive(t *testing.T) {
	baseTimeout := 30 * time.Minute
	strategy := NewSmartTimeStrategy(baseTimeout, 5*time.Minute, false) // extend_if_active = false

	now := time.Now()

	ctx := &Context{
		SessionID:          "session-123",
		SessionStart:       now.Add(-45 * time.Minute),
		LastEventTime:      now.Add(-35 * time.Minute),
		IsLLMActive:        true, // LLM is active
		TimeSinceLastEvent: 35 * time.Minute,
		TimeSinceStart:     45 * time.Minute,
	}

	// With extend_if_active=false, should end even if LLM is active
	result := strategy.ShouldEndSession(ctx)
	if !result {
		t.Errorf("ShouldEndSession() = false, want true (should timeout even with active LLM when extend_if_active=false)")
	}
}
