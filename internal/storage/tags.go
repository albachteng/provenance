package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// PromptTag represents a manual association between a prompt and a commit
type PromptTag struct {
	ID        string
	PromptID  string
	CommitSHA string
	Note      string
	CreatedAt time.Time
}

// CreateTag stores a manual prompt-commit association
// Returns an error (containing "UNIQUE constraint failed") if the tag already exists
func CreateTag(db *sql.DB, tag *PromptTag) error {
	_, err := db.Exec(
		`INSERT INTO prompt_tags (id, prompt_id, commit_sha, note, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		tag.ID, tag.PromptID, tag.CommitSHA, tag.Note, tag.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}
	return nil
}

// GetTagsForPrompt returns all manual tags for a prompt, ordered by creation time
func GetTagsForPrompt(db *sql.DB, promptID string) ([]*PromptTag, error) {
	rows, err := db.Query(
		`SELECT id, prompt_id, commit_sha, note, created_at
		 FROM prompt_tags
		 WHERE prompt_id = ?
		 ORDER BY created_at ASC`,
		promptID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query tags: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var tags []*PromptTag
	for rows.Next() {
		var t PromptTag
		var createdAtUnix int64
		if err := rows.Scan(&t.ID, &t.PromptID, &t.CommitSHA, &t.Note, &createdAtUnix); err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		t.CreatedAt = time.Unix(createdAtUnix, 0)
		tags = append(tags, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tags: %w", err)
	}
	return tags, nil
}

// GetTagsForCommit returns all manually tagged prompts for a commit
func GetTagsForCommit(db *sql.DB, commitSHA string) ([]*PromptTag, error) {
	rows, err := db.Query(
		`SELECT id, prompt_id, commit_sha, note, created_at
		 FROM prompt_tags
		 WHERE commit_sha = ?
		 ORDER BY created_at ASC`,
		commitSHA,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query tags: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var tags []*PromptTag
	for rows.Next() {
		var t PromptTag
		var createdAtUnix int64
		if err := rows.Scan(&t.ID, &t.PromptID, &t.CommitSHA, &t.Note, &createdAtUnix); err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		t.CreatedAt = time.Unix(createdAtUnix, 0)
		tags = append(tags, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tags: %w", err)
	}
	return tags, nil
}

// DeleteTag removes a manual prompt-commit tag
// Returns ErrNotFound if no such tag exists
func DeleteTag(db *sql.DB, promptID, commitSHA string) error {
	result, err := db.Exec(
		`DELETE FROM prompt_tags WHERE prompt_id = ? AND commit_sha = ?`,
		promptID, commitSHA,
	)
	if err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
