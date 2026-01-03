package config

import (
	"fmt"

	"github.com/albachteng/provenance/internal/session"
)

// CreateSessionStrategy creates a SessionStrategy from config
func (c *Config) CreateSessionStrategy() (session.SessionStrategy, error) {
	switch c.Session.Strategy {
	case "smart-time":
		// Dereference pointer booleans with defaults
		extendIfActive := true
		if c.Session.SmartTime.ExtendIfActive != nil {
			extendIfActive = *c.Session.SmartTime.ExtendIfActive
		}

		return session.NewSmartTimeStrategy(
			c.Session.SmartTime.BaseTimeout.Duration,
			c.Session.SmartTime.ActivityCheckInterval.Duration,
			extendIfActive,
		), nil

	case "git-event":
		// Dereference pointer booleans with defaults
		endOnCommit := true
		if c.Session.GitEvent.EndOnCommit != nil {
			endOnCommit = *c.Session.GitEvent.EndOnCommit
		}
		endOnBranchSwitch := true
		if c.Session.GitEvent.EndOnBranchSwitch != nil {
			endOnBranchSwitch = *c.Session.GitEvent.EndOnBranchSwitch
		}

		return session.NewGitEventStrategy(
			endOnCommit,
			endOnBranchSwitch,
			c.Session.GitEvent.FallbackTimeout.Duration,
		), nil

	default:
		return nil, fmt.Errorf("unknown session strategy: %q", c.Session.Strategy)
	}
}
