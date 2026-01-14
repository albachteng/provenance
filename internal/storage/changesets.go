package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ChangeSet represents a correlation between a prompt and code changes
type ChangeSet struct {
	ID                  string
	PromptID            string
	SessionID           string
	Timestamp           time.Time
	FilesChanged        []string
	DiffSummary         string
	CommitIntroduced    string
	CorrelationMethod   string
	Confidence          float64
	TimeToFirstChangeMs int64
}

// CreateChangeSet stores a new change set in the database
func CreateChangeSet(db *sql.DB, cs *ChangeSet) error {
	filesChangedJSON, err := json.Marshal(cs.FilesChanged)
	if err != nil {
		return fmt.Errorf("failed to marshal files_changed: %w", err)
	}

	query := `
		INSERT INTO change_sets (
			id, prompt_id, session_id, timestamp,
			files_changed, diff_summary, commit_introduced,
			correlation_method, confidence, time_to_first_change_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = db.Exec(
		query,
		cs.ID,
		cs.PromptID,
		cs.SessionID,
		cs.Timestamp.Unix(),
		string(filesChangedJSON),
		cs.DiffSummary,
		cs.CommitIntroduced,
		cs.CorrelationMethod,
		cs.Confidence,
		cs.TimeToFirstChangeMs,
	)

	if err != nil {
		return fmt.Errorf("failed to insert change set: %w", err)
	}

	return nil
}

// GetChangeSet retrieves a change set by ID
func GetChangeSet(db *sql.DB, id string) (*ChangeSet, error) {
	query := `
		SELECT id, prompt_id, session_id, timestamp,
		       files_changed, diff_summary, commit_introduced,
		       correlation_method, confidence, time_to_first_change_ms
		FROM change_sets
		WHERE id = ?
	`

	var cs ChangeSet
	var timestampUnix int64
	var filesChangedJSON string

	err := db.QueryRow(query, id).Scan(
		&cs.ID,
		&cs.PromptID,
		&cs.SessionID,
		&timestampUnix,
		&filesChangedJSON,
		&cs.DiffSummary,
		&cs.CommitIntroduced,
		&cs.CorrelationMethod,
		&cs.Confidence,
		&cs.TimeToFirstChangeMs,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query change set: %w", err)
	}

	cs.Timestamp = time.Unix(timestampUnix, 0)

	if err := json.Unmarshal([]byte(filesChangedJSON), &cs.FilesChanged); err != nil {
		return nil, fmt.Errorf("failed to unmarshal files_changed: %w", err)
	}

	return &cs, nil
}

// GetChangeSetsForPrompt retrieves all change sets for a specific prompt
func GetChangeSetsForPrompt(db *sql.DB, promptID string) ([]*ChangeSet, error) {
	query := `
		SELECT id, prompt_id, session_id, timestamp,
		       files_changed, diff_summary, commit_introduced,
		       correlation_method, confidence, time_to_first_change_ms
		FROM change_sets
		WHERE prompt_id = ?
		ORDER BY timestamp ASC
	`

	rows, err := db.Query(query, promptID)
	if err != nil {
		return nil, fmt.Errorf("failed to query change sets: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var changeSets []*ChangeSet

	for rows.Next() {
		var cs ChangeSet
		var timestampUnix int64
		var filesChangedJSON string

		err := rows.Scan(
			&cs.ID,
			&cs.PromptID,
			&cs.SessionID,
			&timestampUnix,
			&filesChangedJSON,
			&cs.DiffSummary,
			&cs.CommitIntroduced,
			&cs.CorrelationMethod,
			&cs.Confidence,
			&cs.TimeToFirstChangeMs,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan change set: %w", err)
		}

		cs.Timestamp = time.Unix(timestampUnix, 0)

		if err := json.Unmarshal([]byte(filesChangedJSON), &cs.FilesChanged); err != nil {
			return nil, fmt.Errorf("failed to unmarshal files_changed: %w", err)
		}

		changeSets = append(changeSets, &cs)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating change sets: %w", err)
	}

	return changeSets, nil
}

// GetChangeSetsForCommit retrieves all change sets for a specific commit, ordered by confidence
func GetChangeSetsForCommit(db *sql.DB, commitSHA string) ([]*ChangeSet, error) {
	query := `
		SELECT id, prompt_id, session_id, timestamp,
		       files_changed, diff_summary, commit_introduced,
		       correlation_method, confidence, time_to_first_change_ms
		FROM change_sets
		WHERE commit_introduced = ?
		ORDER BY confidence DESC
	`

	rows, err := db.Query(query, commitSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to query change sets: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var changeSets []*ChangeSet

	for rows.Next() {
		var cs ChangeSet
		var timestampUnix int64
		var filesChangedJSON string

		err := rows.Scan(
			&cs.ID,
			&cs.PromptID,
			&cs.SessionID,
			&timestampUnix,
			&filesChangedJSON,
			&cs.DiffSummary,
			&cs.CommitIntroduced,
			&cs.CorrelationMethod,
			&cs.Confidence,
			&cs.TimeToFirstChangeMs,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan change set: %w", err)
		}

		cs.Timestamp = time.Unix(timestampUnix, 0)

		if err := json.Unmarshal([]byte(filesChangedJSON), &cs.FilesChanged); err != nil {
			return nil, fmt.Errorf("failed to unmarshal files_changed: %w", err)
		}

		changeSets = append(changeSets, &cs)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating change sets: %w", err)
	}

	return changeSets, nil
}
