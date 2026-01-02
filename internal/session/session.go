package session

import (
	"time"
)

// Session represents an active or completed session
type Session struct {
	ID            string
	StartTime     time.Time
	EndTime       time.Time
	RepoPath      string
	EventCount    int
	LastEventTime time.Time
	IsLLMActive   bool

	GitBranch     string
	GitCommit     string
	LastGitBranch string
	LastGitCommit string
}

func (s *Session) IsActive() bool {
	return s.EndTime.IsZero()
}
