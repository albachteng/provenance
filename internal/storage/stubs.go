package storage

import (
	"database/sql"
	"time"
)

// V2 STUBS: These types and functions are deprecated in v2 architecture
// They are kept as stubs temporarily to allow code compilation
// Session-based commands will be removed or replaced with commit window queries

// Session is deprecated - v2 uses commit windows
type Session struct {
	ID           string
	StartTime    time.Time
	EndTime      *time.Time
	RepoPath     string
	TotalPrompts int
	TotalTokens  int
	EndedBy      string
}

// ChangeSet is deprecated - v2 computes correlations on-demand
type ChangeSet struct {
	ID                string
	CommitSHA         string
	PromptID          string
	PromptEventID     string
	SessionID         string
	Timestamp         time.Time
	CorrelationScore  float64
	CorrelationMethod string
	CommitIntroduced  string
	Confidence        float64
	FilesChanged      []string
	DiffSummary       string
	CreatedAt         time.Time
}

// RepoStats contains aggregate statistics for a repository
type RepoStats struct {
	TotalPrompts   int
	TotalTokensIn  int
	TotalTokensOut int
	SessionCount   int
	FilesMentioned map[string]int
	ToolsInvoked   map[string]int
}

// SessionStats contains statistics for a specific session
type SessionStats struct {
	SessionID      string
	TotalPrompts   int
	TotalTokensIn  int
	TotalTokensOut int
	StartTime      time.Time
	EndTime        *time.Time
	FilesMentioned map[string]int
	ToolsInvoked   map[string]int
}

// CreateSession is deprecated - no-op (for tests)
func CreateSession(db *sql.DB, session *Session) error {
	return nil
}

// ListSessions is deprecated - returns empty list
func ListSessions(db *sql.DB, activeOnly bool) ([]*Session, error) {
	return []*Session{}, nil
}

// GetSession is deprecated - returns not found
func GetSession(db *sql.DB, sessionID string) (*Session, error) {
	return nil, ErrNotFound
}

// EndSession is deprecated - no-op
func EndSession(db *sql.DB, sessionID string, endTime time.Time, reason string) error {
	return nil
}

// GetRepoStats is deprecated - returns empty stats
func GetRepoStats(db *sql.DB, repoPath string) (*RepoStats, error) {
	return &RepoStats{
		FilesMentioned: make(map[string]int),
		ToolsInvoked:   make(map[string]int),
	}, nil
}

// GetSessionStats is deprecated - returns not found
func GetSessionStats(db *sql.DB, sessionID string) (*SessionStats, error) {
	return nil, ErrNotFound
}

// GetTimeframeStats is deprecated - returns empty stats
func GetTimeframeStats(db *sql.DB, repoPath string, since time.Time) (*RepoStats, error) {
	return &RepoStats{
		FilesMentioned: make(map[string]int),
		ToolsInvoked:   make(map[string]int),
	}, nil
}

// GetChangeSetsForCommitPrefix is deprecated - returns empty list
func GetChangeSetsForCommitPrefix(db *sql.DB, commitPrefix string) ([]*ChangeSet, error) {
	return []*ChangeSet{}, nil
}

// GetChangeSetsForFile is deprecated - returns empty list
func GetChangeSetsForFile(db *sql.DB, filePath string) ([]*ChangeSet, error) {
	return []*ChangeSet{}, nil
}

// CreateChangeSet is deprecated - no-op
func CreateChangeSet(db *sql.DB, cs *ChangeSet) error {
	return nil
}

// DeleteManualChangeSet is deprecated - no-op
func DeleteManualChangeSet(db *sql.DB, commitSHA, promptID string) error {
	return nil
}

// GetChangeSetsForCommit is deprecated - returns empty list
func GetChangeSetsForCommit(db *sql.DB, commitSHA string) ([]*ChangeSet, error) {
	return []*ChangeSet{}, nil
}

// GetChangeSetsForPrompt is deprecated - returns empty list
func GetChangeSetsForPrompt(db *sql.DB, promptID string) ([]*ChangeSet, error) {
	return []*ChangeSet{}, nil
}
