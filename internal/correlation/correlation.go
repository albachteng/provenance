package correlation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/albachteng/provenance/internal/storage"
)

// CommitInfo contains information about a git commit for correlation
type CommitInfo struct {
	SHA          string
	Timestamp    time.Time
	RepoPath     string
	FilesChanged []string
	DiffSummary  string
}

// CalculateTimeConfidence calculates confidence based on time delta between prompt and commit
// Returns a value between 0.0 and 1.0, with higher values for smaller time deltas
func CalculateTimeConfidence(promptTime, commitTime time.Time) float64 {
	delta := commitTime.Sub(promptTime)
	if delta < 0 {
		delta = -delta // Handle commits before prompts
	}

	deltaMinutes := delta.Minutes()

	// Piecewise function matching expected ranges
	var confidence float64

	switch {
	case deltaMinutes <= 0.5: // < 30 seconds
		confidence = 0.95 + (0.05 * (1.0 - deltaMinutes/0.5))
	case deltaMinutes < 2.0: // < 2 minutes
		// Linear interpolation from 0.95 to 0.85 (mid-range)
		ratio := (deltaMinutes - 0.5) / 1.5
		confidence = 0.95 - (0.10 * ratio)
	case deltaMinutes < 5.0: // < 5 minutes
		// Linear interpolation from 0.85 to 0.70 (mid-range)
		ratio := (deltaMinutes - 2.0) / 3.0
		confidence = 0.85 - (0.15 * ratio)
	case deltaMinutes < 10.0: // < 10 minutes
		// Linear interpolation from 0.70 to 0.45 (mid-range)
		ratio := (deltaMinutes - 5.0) / 5.0
		confidence = 0.70 - (0.25 * ratio)
	case deltaMinutes < 30.0: // < 30 minutes
		// Linear interpolation from 0.45 to 0.15 (mid-range)
		ratio := (deltaMinutes - 10.0) / 20.0
		confidence = 0.45 - (0.30 * ratio)
	default: // > 30 minutes
		// Exponential decay after 30 minutes
		excessMinutes := deltaMinutes - 30.0
		confidence = 0.15 * math.Exp(-excessMinutes/30.0)
	}

	// Clamp to [0.0, 1.0]
	if confidence < 0.0 {
		confidence = 0.0
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// CalculateFileOverlapConfidence calculates confidence based on file overlap
// Returns a value between 0.0 and 1.0 based on how many mentioned files were changed
func CalculateFileOverlapConfidence(filesMentioned, filesChanged []string) float64 {
	if len(filesMentioned) == 0 {
		// No files mentioned - use low default confidence
		return 0.2
	}

	// Count how many mentioned files appear in the changed files
	matchCount := 0
	for _, mentioned := range filesMentioned {
		for _, changed := range filesChanged {
			if mentioned == changed {
				matchCount++
				break
			}
		}
	}

	// Calculate overlap ratio
	overlapRatio := float64(matchCount) / float64(len(filesMentioned))

	// Check if extra files were changed (mentioned files are subset of changed files)
	allMentionedChanged := (matchCount == len(filesMentioned))
	extraFilesChanged := allMentionedChanged && (len(filesChanged) > len(filesMentioned))

	// Non-linear scaling to match test expectations
	var confidence float64
	if overlapRatio >= 1.0 {
		if extraFilesChanged {
			// All mentioned changed but with extras: 0.70-0.90
			confidence = 0.80
		} else {
			// Perfect overlap (no extras): 0.95-1.0
			confidence = 0.975
		}
	} else if overlapRatio >= 0.60 {
		// High overlap (2/3+): 0.65-0.85, use mid-range 0.75
		confidence = 0.75
	} else if overlapRatio >= 0.30 {
		// Partial overlap (1/3-2/3): 0.30-0.50, use mid-range
		confidence = 0.40
	} else {
		// Low overlap: 0.0-0.20
		confidence = 0.10
	}

	return confidence
}

// CombineConfidenceFactors combines multiple confidence signals into a single score
// Uses weighted average, giving slightly more weight to file overlap than time
func CombineConfidenceFactors(timeConf, fileConf float64) float64 {
	// Weighted average: 40% time, 60% file overlap
	// File overlap is more reliable than time proximity
	timeWeight := 0.4
	fileWeight := 0.6

	combined := (timeConf * timeWeight) + (fileConf * fileWeight)

	// Clamp to [0.0, 1.0]
	if combined < 0.0 {
		combined = 0.0
	}
	if combined > 1.0 {
		combined = 1.0
	}

	return combined
}

// FindRelevantPrompts finds prompts within the time window that might be related to a commit
func FindRelevantPrompts(db *sql.DB, commitInfo *CommitInfo, timeWindow time.Duration) ([]*storage.PromptEvent, error) {
	// Find prompts within time window before the commit, in the same repo
	cutoffTime := commitInfo.Timestamp.Add(-timeWindow)

	query := `
		SELECT id, timestamp, session_id, agent, model_version, prompt_text, response_text,
		       tokens_in, tokens_out, latency_ms, repo_path, git_commit, git_branch, git_dirty,
		       dirty_files, author, ide, active_file, workspace_files, prompt_type,
		       tools_invoked, files_mentioned
		FROM prompt_events
		WHERE repo_path = ?
		  AND timestamp >= ?
		  AND timestamp <= ?
		ORDER BY timestamp DESC
	`

	rows, err := db.Query(query, commitInfo.RepoPath, cutoffTime.Unix(), commitInfo.Timestamp.Unix())
	if err != nil {
		return nil, fmt.Errorf("failed to query prompts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var prompts []*storage.PromptEvent

	for rows.Next() {
		event, err := scanPromptEvent(rows)
		if err != nil {
			return nil, err
		}
		prompts = append(prompts, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating prompts: %w", err)
	}

	return prompts, nil
}

// CorrelateCommitToPrompts creates change sets linking a commit to relevant prompts
func CorrelateCommitToPrompts(db *sql.DB, commitInfo *CommitInfo, timeWindow time.Duration) ([]*storage.ChangeSet, error) {
	// Find relevant prompts
	prompts, err := FindRelevantPrompts(db, commitInfo, timeWindow)
	if err != nil {
		return nil, err
	}

	var changeSets []*storage.ChangeSet

	for _, prompt := range prompts {
		// Calculate confidence
		timeConf := CalculateTimeConfidence(prompt.Timestamp, commitInfo.Timestamp)
		fileConf := CalculateFileOverlapConfidence(prompt.FilesMentioned, commitInfo.FilesChanged)
		combinedConf := CombineConfidenceFactors(timeConf, fileConf)

		// Calculate time to first change
		timeToChange := commitInfo.Timestamp.Sub(prompt.Timestamp)

		// Create change set
		changeSet := &storage.ChangeSet{
			ID:                   fmt.Sprintf("cs-%d-%s", time.Now().UnixNano(), prompt.ID[:8]),
			PromptID:             prompt.ID,
			SessionID:            prompt.SessionID,
			Timestamp:            commitInfo.Timestamp,
			FilesChanged:         commitInfo.FilesChanged,
			DiffSummary:          commitInfo.DiffSummary,
			CommitIntroduced:     commitInfo.SHA,
			CorrelationMethod:    "git_hook",
			Confidence:           combinedConf,
			TimeToFirstChangeMs:  timeToChange.Milliseconds(),
		}

		// Store in database
		if err := storage.CreateChangeSet(db, changeSet); err != nil {
			return nil, fmt.Errorf("failed to create change set: %w", err)
		}

		changeSets = append(changeSets, changeSet)
	}

	return changeSets, nil
}

// scanPromptEvent scans a row into a PromptEvent (helper for FindRelevantPrompts)
func scanPromptEvent(rows *sql.Rows) (*storage.PromptEvent, error) {
	// This is a simplified version - you may want to use the actual implementation
	// from storage package if it's exported
	var event storage.PromptEvent
	var timestampUnix int64
	var modelVersion, responseText, gitCommit, gitBranch, ide, activeFile sql.NullString
	var latencyMs sql.NullInt64
	var gitDirty sql.NullBool
	var dirtyFilesJSON, workspaceFilesJSON, promptType, toolsInvokedJSON, filesMentionedJSON string

	err := rows.Scan(
		&event.ID, &timestampUnix, &event.SessionID, &event.Agent, &modelVersion,
		&event.PromptText, &responseText, &event.TokensIn, &event.TokensOut, &latencyMs,
		&event.RepoPath, &gitCommit, &gitBranch, &gitDirty, &dirtyFilesJSON,
		&event.Author, &ide, &activeFile, &workspaceFilesJSON, &promptType,
		&toolsInvokedJSON, &filesMentionedJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan prompt event: %w", err)
	}

	event.Timestamp = time.Unix(timestampUnix, 0)

	if modelVersion.Valid {
		event.ModelVersion = modelVersion.String
	}
	if responseText.Valid {
		event.ResponseText = responseText.String
	}
	if latencyMs.Valid {
		event.LatencyMs = int(latencyMs.Int64)
	}
	if gitCommit.Valid {
		event.GitCommit = gitCommit.String
	}
	if gitBranch.Valid {
		event.GitBranch = gitBranch.String
	}
	if gitDirty.Valid {
		event.GitDirty = gitDirty.Bool
	}
	if ide.Valid {
		event.IDE = ide.String
	}
	if activeFile.Valid {
		event.ActiveFile = activeFile.String
	}
	if promptType != "" {
		event.PromptType = promptType
	}

	// Parse JSON arrays
	if dirtyFilesJSON != "" {
		json.Unmarshal([]byte(dirtyFilesJSON), &event.DirtyFiles) //nolint:errcheck
	}
	if workspaceFilesJSON != "" {
		json.Unmarshal([]byte(workspaceFilesJSON), &event.WorkspaceFiles) //nolint:errcheck
	}
	if toolsInvokedJSON != "" {
		json.Unmarshal([]byte(toolsInvokedJSON), &event.ToolsInvoked) //nolint:errcheck
	}
	if filesMentionedJSON != "" {
		json.Unmarshal([]byte(filesMentionedJSON), &event.FilesMentioned) //nolint:errcheck
	}

	return &event, nil
}
