package git

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CommitInfo contains information about a git commit
type CommitInfo struct {
	SHA       string
	Timestamp time.Time
	Branch    string
	Author    string
	Message   string
}

// GetCommitTime returns the timestamp of a commit
func GetCommitTime(repoPath, commitSHA string) (time.Time, error) {
	if !isGitRepo(repoPath) {
		return time.Time{}, fmt.Errorf("not a git repository: %s", repoPath)
	}

	output, err := runGitCommand(repoPath, "show", "-s", "--format=%ct", commitSHA)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get commit time: %w", err)
	}

	timestamp, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return time.Unix(timestamp, 0), nil
}

// GetPreviousCommit returns the parent commit on the same branch
// Returns empty string if this is the initial commit
func GetPreviousCommit(repoPath, commitSHA, branch string) (string, error) {
	if !isGitRepo(repoPath) {
		return "", fmt.Errorf("not a git repository: %s", repoPath)
	}

	// Use --first-parent to follow the mainline history
	// Get up to 2 commits starting from commitSHA
	output, err := runGitCommand(repoPath, "rev-list", "--first-parent", "--max-count=2", commitSHA)
	if err != nil {
		return "", fmt.Errorf("failed to get previous commit: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		// This is the initial commit (no parent)
		return "", nil
	}

	return strings.TrimSpace(lines[1]), nil
}

// GetCommitsForFile returns commits that modified a file
func GetCommitsForFile(repoPath, filePath string) ([]CommitInfo, error) {
	if !isGitRepo(repoPath) {
		return nil, fmt.Errorf("not a git repository: %s", repoPath)
	}

	// Use --follow to track file renames
	// Format: SHA|timestamp|refnames|author|subject
	output, err := runGitCommand(repoPath, "log", "--follow", "--format=%H|%ct|%D|%an|%s", "--", filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get commits for file: %w", err)
	}

	commits := []CommitInfo{}
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 2 {
			continue
		}

		timestamp, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}

		branch := extractBranchFromRefs(parts[2])
		author := ""
		message := ""
		if len(parts) >= 4 {
			author = parts[3]
		}
		if len(parts) >= 5 {
			message = parts[4]
		}

		commits = append(commits, CommitInfo{
			SHA:       parts[0],
			Timestamp: time.Unix(timestamp, 0),
			Branch:    branch,
			Author:    author,
			Message:   message,
		})
	}

	return commits, nil
}

// GetFilesChanged returns the list of files changed in a commit
func GetFilesChanged(repoPath, commitSHA string) ([]string, error) {
	if !isGitRepo(repoPath) {
		return nil, fmt.Errorf("not a git repository: %s", repoPath)
	}

	output, err := runGitCommand(repoPath, "show", "--name-only", "--format=", commitSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get files changed: %w", err)
	}

	files := []string{}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}

	return files, nil
}

// GetCurrentBranch returns the current branch name
func GetCurrentBranch(repoPath string) (string, error) {
	if !isGitRepo(repoPath) {
		return "", fmt.Errorf("not a git repository: %s", repoPath)
	}

	output, err := runGitCommand(repoPath, "branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	return strings.TrimSpace(output), nil
}

// GetBranchForCommit tries to determine which branch a commit belongs to
// Returns the first branch found that contains the commit
func GetBranchForCommit(repoPath, commitSHA string) (string, error) {
	if !isGitRepo(repoPath) {
		return "", fmt.Errorf("not a git repository: %s", repoPath)
	}

	// Get all branches containing this commit
	output, err := runGitCommand(repoPath, "branch", "--contains", commitSHA)
	if err != nil {
		return "", fmt.Errorf("failed to get branch for commit: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("no branch found for commit %s", commitSHA)
	}

	// Return the first branch (remove * prefix if it's the current branch)
	branch := strings.TrimSpace(lines[0])
	branch = strings.TrimPrefix(branch, "* ")

	return branch, nil
}

// GetCommitsInBranch returns all commits in a branch
func GetCommitsInBranch(repoPath, branch string) ([]CommitInfo, error) {
	if !isGitRepo(repoPath) {
		return nil, fmt.Errorf("not a git repository: %s", repoPath)
	}

	// Format: SHA|timestamp|author|subject
	output, err := runGitCommand(repoPath, "log", "--first-parent", "--format=%H|%ct|%an|%s", branch)
	if err != nil {
		return nil, fmt.Errorf("failed to get commits in branch: %w", err)
	}

	commits := []CommitInfo{}
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 2 {
			continue
		}

		timestamp, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}

		author := ""
		message := ""
		if len(parts) >= 3 {
			author = parts[2]
		}
		if len(parts) >= 4 {
			message = parts[3]
		}

		commits = append(commits, CommitInfo{
			SHA:       parts[0],
			Timestamp: time.Unix(timestamp, 0),
			Branch:    branch,
			Author:    author,
			Message:   message,
		})
	}

	return commits, nil
}

// extractBranchFromRefs parses git refs like "HEAD -> main, origin/main" and returns the branch name
func extractBranchFromRefs(refs string) string {
	if refs == "" {
		return ""
	}

	// Split by comma to handle multiple refs
	parts := strings.Split(refs, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Skip HEAD pointer
		if strings.HasPrefix(part, "HEAD -> ") {
			branch := strings.TrimPrefix(part, "HEAD -> ")
			return strings.TrimSpace(branch)
		}

		// Skip remote refs
		if strings.Contains(part, "/") {
			continue
		}

		// Skip tags
		if strings.HasPrefix(part, "tag:") {
			continue
		}

		// Return first non-remote, non-tag ref
		if part != "HEAD" && part != "" {
			return part
		}
	}

	return ""
}
