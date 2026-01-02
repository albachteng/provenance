package config

import (
	"testing"
	"time"
)

func TestCreateSessionStrategy_SmartTime(t *testing.T) {
	cfg := Default()
	cfg.Session.Strategy = "smart-time"
	cfg.Session.SmartTime.BaseTimeout = Duration{45 * time.Minute}
	cfg.Session.SmartTime.ActivityCheckInterval = Duration{10 * time.Minute}
	extendIfActive := true
	cfg.Session.SmartTime.ExtendIfActive = &extendIfActive

	strategy, err := cfg.CreateSessionStrategy()
	if err != nil {
		t.Fatalf("CreateSessionStrategy() failed: %v", err)
	}

	if strategy == nil {
		t.Fatal("expected strategy, got nil")
	}

	if strategy.Name() != "smart-time" {
		t.Errorf("expected strategy name 'smart-time', got %q", strategy.Name())
	}
}

func TestCreateSessionStrategy_GitEvent(t *testing.T) {
	cfg := Default()
	cfg.Session.Strategy = "git-event"
	endOnCommit := true
	endOnBranchSwitch := true
	cfg.Session.GitEvent.EndOnCommit = &endOnCommit
	cfg.Session.GitEvent.EndOnBranchSwitch = &endOnBranchSwitch
	cfg.Session.GitEvent.FallbackTimeout = Duration{2 * time.Hour}

	strategy, err := cfg.CreateSessionStrategy()
	if err != nil {
		t.Fatalf("CreateSessionStrategy() failed: %v", err)
	}

	if strategy == nil {
		t.Fatal("expected strategy, got nil")
	}

	if strategy.Name() != "git-event" {
		t.Errorf("expected strategy name 'git-event', got %q", strategy.Name())
	}
}

func TestCreateSessionStrategy_InvalidStrategy(t *testing.T) {
	cfg := Default()
	cfg.Session.Strategy = "invalid-strategy"

	_, err := cfg.CreateSessionStrategy()
	if err == nil {
		t.Error("expected error for invalid strategy, got nil")
	}

	if err != nil && !contains(err.Error(), "unknown session strategy") {
		t.Errorf("expected error about unknown strategy, got: %v", err)
	}
}

func TestCreateSessionStrategy_DefaultValues(t *testing.T) {
	cfg := Default()

	strategy, err := cfg.CreateSessionStrategy()
	if err != nil {
		t.Fatalf("CreateSessionStrategy() failed: %v", err)
	}

	// Default strategy should be smart-time
	if strategy.Name() != "smart-time" {
		t.Errorf("expected default strategy 'smart-time', got %q", strategy.Name())
	}
}
