package correlation

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/albachteng/provenance/internal/storage"
)

// TestCalculateTimeConfidence tests confidence scoring based on time delta
func TestCalculateTimeConfidence(t *testing.T) {
	tests := []struct {
		name          string
		promptTime    time.Time
		commitTime    time.Time
		expectedRange [2]float64 // min, max confidence
		description   string
	}{
		{
			name:          "immediate (1 second)",
			promptTime:    time.Now(),
			commitTime:    time.Now().Add(1 * time.Second),
			expectedRange: [2]float64{0.95, 1.0},
			description:   "Very recent prompts should have very high confidence",
		},
		{
			name:          "very recent (30 seconds)",
			promptTime:    time.Now(),
			commitTime:    time.Now().Add(30 * time.Second),
			expectedRange: [2]float64{0.90, 1.0},
			description:   "Prompts within 30s should have high confidence",
		},
		{
			name:          "recent (2 minutes)",
			promptTime:    time.Now(),
			commitTime:    time.Now().Add(2 * time.Minute),
			expectedRange: [2]float64{0.80, 0.95},
			description:   "Prompts within 2min should have good confidence",
		},
		{
			name:          "moderate (5 minutes)",
			promptTime:    time.Now(),
			commitTime:    time.Now().Add(5 * time.Minute),
			expectedRange: [2]float64{0.60, 0.85},
			description:   "Prompts within 5min should have moderate confidence",
		},
		{
			name:          "old (10 minutes)",
			promptTime:    time.Now(),
			commitTime:    time.Now().Add(10 * time.Minute),
			expectedRange: [2]float64{0.30, 0.65},
			description:   "Prompts from 10min ago should have lower confidence",
		},
		{
			name:          "very old (30 minutes)",
			promptTime:    time.Now(),
			commitTime:    time.Now().Add(30 * time.Minute),
			expectedRange: [2]float64{0.0, 0.35},
			description:   "Prompts from 30min ago should have very low confidence",
		},
		{
			name:          "ancient (1 hour)",
			promptTime:    time.Now(),
			commitTime:    time.Now().Add(1 * time.Hour),
			expectedRange: [2]float64{0.0, 0.15},
			description:   "Prompts from 1hr ago should have minimal confidence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := CalculateTimeConfidence(tt.promptTime, tt.commitTime)

			if confidence < tt.expectedRange[0] || confidence > tt.expectedRange[1] {
				t.Errorf("CalculateTimeConfidence() = %f, want range [%f, %f] - %s",
					confidence, tt.expectedRange[0], tt.expectedRange[1], tt.description)
			}
		})
	}
}

// TestCalculateFileOverlapConfidence tests confidence scoring based on file overlap
func TestCalculateFileOverlapConfidence(t *testing.T) {
	tests := []struct {
		name           string
		filesMentioned []string
		filesChanged   []string
		expectedRange  [2]float64
		description    string
	}{
		{
			name:           "perfect overlap",
			filesMentioned: []string{"auth.go", "auth_test.go"},
			filesChanged:   []string{"auth.go", "auth_test.go"},
			expectedRange:  [2]float64{0.95, 1.0},
			description:    "All mentioned files changed should be very high confidence",
		},
		{
			name:           "high overlap (2/3)",
			filesMentioned: []string{"auth.go", "handler.go", "utils.go"},
			filesChanged:   []string{"auth.go", "handler.go"},
			expectedRange:  [2]float64{0.65, 0.85},
			description:    "Most mentioned files changed should be good confidence",
		},
		{
			name:           "partial overlap (1/3)",
			filesMentioned: []string{"auth.go", "handler.go", "utils.go"},
			filesChanged:   []string{"auth.go"},
			expectedRange:  [2]float64{0.30, 0.50},
			description:    "Some mentioned files changed should be moderate confidence",
		},
		{
			name:           "no overlap",
			filesMentioned: []string{"auth.go", "handler.go"},
			filesChanged:   []string{"utils.go", "config.go"},
			expectedRange:  [2]float64{0.0, 0.2},
			description:    "No mentioned files changed should be very low confidence",
		},
		{
			name:           "no files mentioned",
			filesMentioned: []string{},
			filesChanged:   []string{"auth.go", "handler.go"},
			expectedRange:  [2]float64{0.0, 0.3},
			description:    "No files mentioned should give low default confidence",
		},
		{
			name:           "extra files changed",
			filesMentioned: []string{"auth.go"},
			filesChanged:   []string{"auth.go", "handler.go", "utils.go", "config.go"},
			expectedRange:  [2]float64{0.70, 0.90},
			description:    "All mentioned files changed (plus extras) should be high confidence",
		},
		{
			name:           "path variations",
			filesMentioned: []string{"internal/auth/auth.go", "internal/auth/handler.go"},
			filesChanged:   []string{"internal/auth/auth.go", "internal/auth/handler.go"},
			expectedRange:  [2]float64{0.95, 1.0},
			description:    "Should handle full paths correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := CalculateFileOverlapConfidence(tt.filesMentioned, tt.filesChanged)

			if confidence < tt.expectedRange[0] || confidence > tt.expectedRange[1] {
				t.Errorf("CalculateFileOverlapConfidence() = %f, want range [%f, %f] - %s",
					confidence, tt.expectedRange[0], tt.expectedRange[1], tt.description)
			}
		})
	}
}

// TestCombineConfidenceFactors tests combining multiple confidence signals
func TestCombineConfidenceFactors(t *testing.T) {
	tests := []struct {
		name          string
		timeConf      float64
		fileConf      float64
		expectedRange [2]float64
		description   string
	}{
		{
			name:          "both high",
			timeConf:      0.95,
			fileConf:      0.90,
			expectedRange: [2]float64{0.90, 1.0},
			description:   "High confidence from both factors should be very high overall",
		},
		{
			name:          "time high, files low",
			timeConf:      0.90,
			fileConf:      0.20,
			expectedRange: [2]float64{0.40, 0.65},
			description:   "Should balance both factors",
		},
		{
			name:          "time low, files high",
			timeConf:      0.20,
			fileConf:      0.90,
			expectedRange: [2]float64{0.40, 0.65},
			description:   "Should balance both factors",
		},
		{
			name:          "both moderate",
			timeConf:      0.60,
			fileConf:      0.50,
			expectedRange: [2]float64{0.50, 0.65},
			description:   "Moderate factors should give moderate overall",
		},
		{
			name:          "both low",
			timeConf:      0.10,
			fileConf:      0.15,
			expectedRange: [2]float64{0.0, 0.20},
			description:   "Low factors should give low overall",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := CombineConfidenceFactors(tt.timeConf, tt.fileConf)

			if confidence < tt.expectedRange[0] || confidence > tt.expectedRange[1] {
				t.Errorf("CombineConfidenceFactors(%f, %f) = %f, want range [%f, %f] - %s",
					tt.timeConf, tt.fileConf, confidence, tt.expectedRange[0], tt.expectedRange[1], tt.description)
			}

			// Combined confidence should never exceed individual factors
			maxInput := tt.timeConf
			if tt.fileConf > maxInput {
				maxInput = tt.fileConf
			}
			if confidence > maxInput+0.05 { // Small tolerance for rounding
				t.Errorf("Combined confidence %f should not significantly exceed max input %f", confidence, maxInput)
			}
		})
	}
}

// TestFindRelevantPrompts tests finding prompts that might be related to a commit
func TestFindRelevantPrompts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now()

	// Create sessions for both test repo and different repo
	session1 := &storage.Session{
		ID:        "session-123",
		StartTime: now.Add(-20 * time.Minute),
		RepoPath:  "/test/repo",
	}
	err := storage.CreateSession(db, session1)
	if err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	session2 := &storage.Session{
		ID:        "session-456",
		StartTime: now.Add(-5 * time.Minute),
		RepoPath:  "/other/repo",
	}
	err = storage.CreateSession(db, session2)
	if err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	// Create prompts at different times
	prompts := []*storage.PromptEvent{
		{
			ID:             "evt-recent",
			Timestamp:      now.Add(-2 * time.Minute),
			SessionID:      "session-123",
			Agent:          "claude-code",
			PromptText:     "Add authentication",
			RepoPath:       "/test/repo",
			Author:         "testuser",
			FilesMentioned: []string{"auth.go"},
		},
		{
			ID:             "evt-old",
			Timestamp:      now.Add(-25 * time.Minute),
			SessionID:      "session-123",
			Agent:          "claude-code",
			PromptText:     "Setup project",
			RepoPath:       "/test/repo",
			Author:         "testuser",
			FilesMentioned: []string{"main.go"},
		},
		{
			ID:             "evt-different-repo",
			Timestamp:      now.Add(-1 * time.Minute),
			SessionID:      "session-456",
			Agent:          "claude-code",
			PromptText:     "Different repo",
			RepoPath:       "/other/repo",
			Author:         "testuser",
			FilesMentioned: []string{},
		},
	}

	for _, prompt := range prompts {
		err := storage.StorePromptEvent(db, prompt)
		if err != nil {
			t.Fatalf("Failed to store prompt: %v", err)
		}
	}

	// Find relevant prompts for commit at current time in /test/repo
	commitInfo := &CommitInfo{
		SHA:          "abc123",
		Timestamp:    now,
		RepoPath:     "/test/repo",
		FilesChanged: []string{"auth.go", "auth_test.go"},
	}

	relevant, err := FindRelevantPrompts(db, commitInfo, 15*time.Minute)
	if err != nil {
		t.Fatalf("FindRelevantPrompts failed: %v", err)
	}

	// Should find evt-recent (within time window, same repo)
	// Should NOT find evt-old (outside time window)
	// Should NOT find evt-different-repo (different repo)
	if len(relevant) != 1 {
		t.Fatalf("Expected 1 relevant prompt, got %d", len(relevant))
	}

	if relevant[0].ID != "evt-recent" {
		t.Errorf("Expected to find evt-recent, got %s", relevant[0].ID)
	}
}

// TestCorrelateCommitToPrompts tests the full correlation workflow
func TestCorrelateCommitToPrompts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now()

	// Create session
	session := &storage.Session{
		ID:        "session-123",
		StartTime: now.Add(-10 * time.Minute),
		RepoPath:  "/test/repo",
	}
	err := storage.CreateSession(db, session)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Create prompts

	prompts := []*storage.PromptEvent{
		{
			ID:             "evt-high-conf",
			Timestamp:      now.Add(-1 * time.Minute),
			SessionID:      "session-123",
			Agent:          "claude-code",
			PromptText:     "Implement user login",
			RepoPath:       "/test/repo",
			Author:         "testuser",
			FilesMentioned: []string{"auth.go", "login.go"},
		},
		{
			ID:             "evt-low-conf",
			Timestamp:      now.Add(-8 * time.Minute),
			SessionID:      "session-123",
			Agent:          "claude-code",
			PromptText:     "Setup database",
			RepoPath:       "/test/repo",
			Author:         "testuser",
			FilesMentioned: []string{"db.go"},
		},
	}

	for _, prompt := range prompts {
		err := storage.StorePromptEvent(db, prompt)
		if err != nil {
			t.Fatalf("Failed to store prompt: %v", err)
		}
	}

	// Correlate a commit
	commitInfo := &CommitInfo{
		SHA:          "commit-abc123",
		Timestamp:    now,
		RepoPath:     "/test/repo",
		FilesChanged: []string{"auth.go", "login.go", "utils.go"},
		DiffSummary:  "+200 -50",
	}

	changeSets, err := CorrelateCommitToPrompts(db, commitInfo, 15*time.Minute)
	if err != nil {
		t.Fatalf("CorrelateCommitToPrompts failed: %v", err)
	}

	// Should create change sets for both prompts
	if len(changeSets) != 2 {
		t.Fatalf("Expected 2 change sets, got %d", len(changeSets))
	}

	// Find the high confidence one
	var highConf *storage.ChangeSet
	for _, cs := range changeSets {
		if cs.PromptID == "evt-high-conf" {
			highConf = cs
			break
		}
	}

	if highConf == nil {
		t.Fatal("Expected to find change set for evt-high-conf")
	}

	// Verify high confidence change set
	if highConf.Confidence < 0.70 {
		t.Errorf("High confidence change set has confidence %f, want >= 0.70", highConf.Confidence)
	}

	if highConf.CommitIntroduced != "commit-abc123" {
		t.Errorf("CommitIntroduced = %s, want commit-abc123", highConf.CommitIntroduced)
	}

	if len(highConf.FilesChanged) != 3 {
		t.Errorf("FilesChanged length = %d, want 3", len(highConf.FilesChanged))
	}

	if highConf.DiffSummary != "+200 -50" {
		t.Errorf("DiffSummary = %s, want +200 -50", highConf.DiffSummary)
	}

	if highConf.CorrelationMethod != "git_hook" {
		t.Errorf("CorrelationMethod = %s, want git_hook", highConf.CorrelationMethod)
	}

	// Verify change sets were persisted to database
	retrieved, err := storage.GetChangeSetsForCommit(db, "commit-abc123")
	if err != nil {
		t.Fatalf("Failed to retrieve change sets: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("Expected 2 persisted change sets, got %d", len(retrieved))
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := storage.InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	return db
}
