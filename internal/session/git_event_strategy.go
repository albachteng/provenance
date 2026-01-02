package session

import (
	"time"
)

// GitEventStrategy implements git-based session boundaries
type GitEventStrategy struct {
	endOnCommit       bool
	endOnBranchSwitch bool
	fallbackTimeout   time.Duration
}

func NewGitEventStrategy(endOnCommit, endOnBranchSwitch bool, fallbackTimeout time.Duration) *GitEventStrategy {
	return &GitEventStrategy{
		endOnCommit:       endOnCommit,
		endOnBranchSwitch: endOnBranchSwitch,
		fallbackTimeout:   fallbackTimeout,
	}
}

func (s *GitEventStrategy) Name() string {
	return "git-event"
}

func (s *GitEventStrategy) ShouldStartSession(ctx *Context) bool {
	return ctx.SessionID == ""
}

func (s *GitEventStrategy) ShouldEndSession(ctx *Context) bool {
	if ctx.SessionID == "" {
		return false
	}

	if s.endOnCommit && ctx.LastGitCommit != "" && ctx.GitCommit != ctx.LastGitCommit {
		return true
	}

	if s.endOnBranchSwitch && ctx.LastGitBranch != "" && ctx.GitBranch != ctx.LastGitBranch {
		return true
	}

	if ctx.TimeSinceStart >= s.fallbackTimeout {
		return true
	}

	return false
}
