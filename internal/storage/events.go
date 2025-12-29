package storage

import "time"

// PromptEvent represents a single AI interaction captured for provenance
type PromptEvent struct {
	ID        string
	Timestamp time.Time
	SessionID string

	// AI metadata
	Agent        string
	ModelVersion string
	PromptText   string
	ResponseText string

	// Usage metrics
	TokensIn  int
	TokensOut int
	LatencyMs int

	// Git context
	RepoPath   string
	GitCommit  string
	GitBranch  string
	GitDirty   bool
	DirtyFiles []string // JSON array in DB

	// Developer context
	Author         string
	IDE            string
	ActiveFile     string
	WorkspaceFiles []string // JSON array in DB

	// Categorization
	PromptType     string
	ToolsInvoked   []string // JSON array in DB
	FilesMentioned []string // JSON array in DB
}

// Session represents a group of related prompts
type Session struct {
	ID           string
	StartTime    time.Time
	EndTime      *time.Time // nil if active
	RepoPath     string
	TotalPrompts int
	TotalTokens  int
	EndedBy      string // 'commit' | 'timeout' | 'manual'
}
