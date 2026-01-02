package config

import (
	"fmt"

	"github.com/albachteng/provenance/internal/session"
)

// CreateSessionStrategy creates a SessionStrategy from config
func (c *Config) CreateSessionStrategy() (session.SessionStrategy, error) {
	switch c.Session.Strategy {
	case "smart-time":
		return session.NewSmartTimeStrategy(
			c.Session.SmartTime.BaseTimeout.Duration,
			c.Session.SmartTime.ActivityCheckInterval.Duration,
			c.Session.SmartTime.ExtendIfActive,
		), nil

	case "git-event":
		return session.NewGitEventStrategy(
			c.Session.GitEvent.EndOnCommit,
			c.Session.GitEvent.EndOnBranchSwitch,
			c.Session.GitEvent.FallbackTimeout.Duration,
		), nil

	default:
		return nil, fmt.Errorf("unknown session strategy: %q", c.Session.Strategy)
	}
}
