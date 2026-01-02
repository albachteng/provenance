package session

import (
	"time"
)

// SessionStrategy determines when to start and end sessions
type SessionStrategy interface {
	// ShouldStartSession returns true if a new session should be created
	ShouldStartSession(ctx *Context) bool

	// ShouldEndSession returns true if the current session should end
	ShouldEndSession(ctx *Context) bool

	// Name returns the strategy name for configuration
	Name() string
}

// Context provides information for session boundary decisions
type Context struct {
	// Current session info
	SessionID       string
	SessionStart    time.Time
	LastEventTime   time.Time
	EventCount      int
	IsLLMActive     bool
	CurrentRepoPath string

	// Git context
	GitBranch     string
	GitCommit     string
	GitDirty      bool
	LastGitBranch string // Branch at session start
	LastGitCommit string // Commit at last check

	// Activity tracking
	TimeSinceLastEvent time.Duration
	TimeSinceStart     time.Duration
}

// EndReason describes why a session ended
type EndReason string

const (
	EndReasonTimeout      EndReason = "timeout"
	EndReasonInactivity   EndReason = "inactivity"
	EndReasonCommit       EndReason = "git_commit"
	EndReasonBranchSwitch EndReason = "git_branch_switch"
	EndReasonManual       EndReason = "manual"
)
