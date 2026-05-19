package git

import (
	"fmt"
	"os/exec"
	"time"
)

// InitRepo initializes a new git repository
func InitRepo(repoPath string) error {
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init failed: %w: %s", err, output)
	}

	// Configure git for testing (avoid warnings about user identity)
	configCmds := [][]string{
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
		{"config", "commit.gpgsign", "false"},
	}

	for _, args := range configCmds {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git config failed: %w: %s", err, output)
		}
	}

	return nil
}

// StageFiles stages files for commit
func StageFiles(repoPath string, files []string) error {
	args := append([]string{"add"}, files...)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w: %s", err, output)
	}
	return nil
}

// CreateCommitWithTime creates a commit with a specific timestamp
func CreateCommitWithTime(repoPath, message string, commitTime time.Time) (string, error) {
	// Set GIT_AUTHOR_DATE and GIT_COMMITTER_DATE environment variables
	timeStr := commitTime.Format(time.RFC3339)

	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoPath
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("GIT_AUTHOR_DATE=%s", timeStr),
		fmt.Sprintf("GIT_COMMITTER_DATE=%s", timeStr),
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit failed: %w: %s", err, output)
	}

	// Get the commit SHA
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}

	return string(output[:40]), nil // Return first 40 chars (full SHA)
}
