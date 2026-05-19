package git

import (
	"testing"
)

// TestBlameLines smoke test
func TestBlameLines(t *testing.T) {
	repoPath := "../.."

	// Blame lines 1-5 of go.mod (should always exist)
	blameLines, err := BlameLines(repoPath, "go.mod", 1, 5)
	if err != nil {
		t.Fatalf("Failed to blame lines: %v", err)
	}

	if len(blameLines) == 0 {
		t.Error("Expected at least one blame line")
	}

	// Verify blame info is populated
	for i, line := range blameLines {
		if line.Commit == "" {
			t.Errorf("Line %d has empty commit SHA", i)
		}
		if len(line.Commit) != 40 {
			t.Errorf("Line %d has invalid commit SHA length: %d", i, len(line.Commit))
		}
		if line.Line != i+1 {
			t.Errorf("Line %d has wrong line number: %d", i, line.Line)
		}
	}
}

// TestBlameLinesInvalidRange tests error handling
func TestBlameLinesInvalidRange(t *testing.T) {
	repoPath := "../.."

	_, err := BlameLines(repoPath, "go.mod", 5, 1) // End before start
	if err == nil {
		t.Error("Expected error for invalid line range")
	}

	_, err = BlameLines(repoPath, "go.mod", 0, 5) // Start < 1
	if err == nil {
		t.Error("Expected error for line range starting at 0")
	}
}

// TestBlameLinesNonexistentFile tests error handling
func TestBlameLinesNonexistentFile(t *testing.T) {
	repoPath := "../.."

	_, err := BlameLines(repoPath, "nonexistent-file.txt", 1, 5)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

// TestGetCommitsForLines smoke test
func TestGetCommitsForLines(t *testing.T) {
	repoPath := "../.."

	commits, err := GetCommitsForLines(repoPath, "go.mod", 1, 10)
	if err != nil {
		t.Fatalf("Failed to get commits for lines: %v", err)
	}

	if len(commits) == 0 {
		t.Error("Expected at least one commit for go.mod lines")
	}

	// Verify all commits are valid SHAs
	for _, commit := range commits {
		if len(commit) != 40 {
			t.Errorf("Invalid commit SHA: %s (length %d)", commit, len(commit))
		}
	}

	// Verify deduplication (should have unique commits only)
	seen := make(map[string]bool)
	for _, commit := range commits {
		if seen[commit] {
			t.Errorf("Duplicate commit found: %s", commit)
		}
		seen[commit] = true
	}
}

// TestParseBlameOutput tests the blame output parser
func TestParseBlameOutput(t *testing.T) {
	// Sample git blame --porcelain output
	sampleOutput := `abc1234567890123456789012345678901234567 1 1 1
author John Doe
author-mail <john@example.com>
author-time 1234567890
author-tz +0000
committer John Doe
committer-mail <john@example.com>
committer-time 1234567890
committer-tz +0000
summary Initial commit
filename test.go
	package main
def9876543210987654321098765432109876543 2 2 1
author Jane Smith
author-mail <jane@example.com>
author-time 1234567891
author-tz +0000
committer Jane Smith
committer-mail <jane@example.com>
committer-time 1234567891
committer-tz +0000
summary Add feature
filename test.go
	import "fmt"
`

	lines := parseBlameOutput(sampleOutput, 1)

	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines, got %d", len(lines))
	}

	// Check first line
	if lines[0].Commit != "abc1234567890123456789012345678901234567" {
		t.Errorf("First line commit = %s, want abc1234567890123456789012345678901234567", lines[0].Commit)
	}
	if lines[0].Content != "package main" {
		t.Errorf("First line content = %q, want %q", lines[0].Content, "package main")
	}
	if lines[0].Line != 1 {
		t.Errorf("First line number = %d, want 1", lines[0].Line)
	}

	// Check second line
	if lines[1].Commit != "def9876543210987654321098765432109876543" {
		t.Errorf("Second line commit = %s, want def9876543210987654321098765432109876543", lines[1].Commit)
	}
	if lines[1].Content != "import \"fmt\"" {
		t.Errorf("Second line content = %q, want %q", lines[1].Content, "import \"fmt\"")
	}
	if lines[1].Line != 2 {
		t.Errorf("Second line number = %d, want 2", lines[1].Line)
	}
}
