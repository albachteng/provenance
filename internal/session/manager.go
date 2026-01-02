package session

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/albachteng/provenance/internal/storage"
)

// Manager manages session lifecycle and boundaries
type Manager struct {
	db             *sql.DB
	strategy       SessionStrategy
	currentSession *Session
}

// NewManager creates a new session manager
func NewManager(db *sql.DB, strategy SessionStrategy) *Manager {
	return &Manager{
		db:       db,
		strategy: strategy,
	}
}

// StartSession creates and starts a new session
func (m *Manager) StartSession(repoPath string) (string, error) {
	// Check if a session is already active
	if m.currentSession != nil && m.currentSession.IsActive() {
		return "", fmt.Errorf("session already active: %s", m.currentSession.ID)
	}

	// Generate session ID
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())

	// Create session
	now := time.Now()
	session := &Session{
		ID:            sessionID,
		StartTime:     now,
		RepoPath:      repoPath,
		EventCount:    0,
		LastEventTime: now,
		IsLLMActive:   false,
	}

	// Persist to database
	dbSession := storage.Session{
		ID:        sessionID,
		StartTime: now,
		RepoPath:  repoPath,
	}

	if err := storage.CreateSession(m.db, &dbSession); err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	m.currentSession = session
	return sessionID, nil
}

// EndSession ends the current session
func (m *Manager) EndSession(sessionID string, reason EndReason) error {
	if m.currentSession == nil || m.currentSession.ID != sessionID {
		return fmt.Errorf("session %s is not active", sessionID)
	}

	// Set end time
	endTime := time.Now()
	m.currentSession.EndTime = endTime

	// Update in database
	if err := storage.EndSession(m.db, sessionID, endTime, string(reason)); err != nil {
		return fmt.Errorf("failed to end session: %w", err)
	}

	// Clear current session
	m.currentSession = nil
	return nil
}

// GetCurrentSession returns the active session or nil
func (m *Manager) GetCurrentSession() *Session {
	if m.currentSession != nil && m.currentSession.IsActive() {
		return m.currentSession
	}
	return nil
}

// RecordEvent records an event in the current session
func (m *Manager) RecordEvent(sessionID string, eventID string) error {
	if m.currentSession == nil || m.currentSession.ID != sessionID {
		return fmt.Errorf("session %s is not active", sessionID)
	}

	// Update activity tracking
	m.currentSession.EventCount++
	m.currentSession.LastEventTime = time.Now()

	return nil
}

// SetLLMActive sets the LLM activity state
func (m *Manager) SetLLMActive(active bool) {
	if m.currentSession != nil {
		m.currentSession.IsLLMActive = active
	}
}

// CheckSessionBoundaries checks if the current session should end based on strategy
func (m *Manager) CheckSessionBoundaries() bool {
	if m.currentSession == nil {
		return false
	}

	// Build context
	ctx := &Context{
		SessionID:          m.currentSession.ID,
		SessionStart:       m.currentSession.StartTime,
		LastEventTime:      m.currentSession.LastEventTime,
		EventCount:         m.currentSession.EventCount,
		IsLLMActive:        m.currentSession.IsLLMActive,
		CurrentRepoPath:    m.currentSession.RepoPath,
		GitBranch:          m.currentSession.GitBranch,
		GitCommit:          m.currentSession.GitCommit,
		LastGitBranch:      m.currentSession.LastGitBranch,
		LastGitCommit:      m.currentSession.LastGitCommit,
		TimeSinceLastEvent: time.Since(m.currentSession.LastEventTime),
		TimeSinceStart:     time.Since(m.currentSession.StartTime),
	}

	// Check if strategy says to end
	if m.strategy.ShouldEndSession(ctx) {
		// End the session
		if err := m.EndSession(m.currentSession.ID, EndReasonTimeout); err != nil {
			return false
		}
		return true
	}

	return false
}
