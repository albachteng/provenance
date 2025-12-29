package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// CLIProvider implements Provider using os/exec and git CLI commands
type CLIProvider struct{}

// CaptureState captures Git repository state using git CLI commands
func (p *CLIProvider) CaptureState(repoPath string) (*GitState, error) {
	// Validate that this is a git repository
	if !isGitRepo(repoPath) {
		return nil, fmt.Errorf("not a git repository: %s", repoPath)
	}

	state := &GitState{
		RepoPath: repoPath,
	}

	// Capture HEAD commit hash
	head, err := runGitCommand(repoPath, "rev-parse", "HEAD")
	if err != nil {
		// Empty repo might not have HEAD yet
		if !strings.Contains(err.Error(), "unknown revision") &&
			!strings.Contains(err.Error(), "ambiguous argument") {
			return nil, fmt.Errorf("failed to get HEAD: %w", err)
		}
		// For empty repos, HEAD doesn't exist yet - this is OK
	} else {
		state.Head = strings.TrimSpace(head)
	}

	// Capture current branch name
	branch, err := runGitCommand(repoPath, "branch", "--show-current")
	if err == nil {
		state.Branch = strings.TrimSpace(branch)
	}
	// If error or empty (detached HEAD), branch might be empty - this is OK

	// Check dirty state using porcelain status
	statusOutput, err := runGitCommand(repoPath, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	statusOutput = strings.TrimSpace(statusOutput)
	state.IsDirty = len(statusOutput) > 0
	if state.IsDirty {
		state.DirtyFiles = parseDirtyFiles(statusOutput)
	}

	// Generate diff summary if dirty
	if state.IsDirty {
		diffStat, err := runGitCommand(repoPath, "diff", "--stat", "HEAD")
		if err == nil {
			state.DiffSummary = strings.TrimSpace(diffStat)
		}
	}

	// Capture remote tracking branch
	remoteTracking, err := runGitCommand(repoPath, "rev-parse", "--abbrev-ref", "@{upstream}")
	if err == nil {
		state.RemoteTracking = strings.TrimSpace(remoteTracking)
	}
	// No error if no upstream is set - this is normal for local-only repos

	// Calculate ahead/behind if we have a remote tracking branch
	if state.RemoteTracking != "" && state.Head != "" {
		aheadBehind, err := runGitCommand(repoPath, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
		if err == nil {
			ahead, behind := parseAheadBehind(strings.TrimSpace(aheadBehind))
			state.Ahead = ahead
			state.Behind = behind
		}
	}

	return state, nil
}

// isGitRepo checks if the given path is a git repository
func isGitRepo(path string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = path
	err := cmd.Run()
	return err == nil
}

// runGitCommand executes a git command in the specified directory
func runGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git command failed: %w (output: %s)", err, string(output))
	}
	return string(output), nil
}

// parseDirtyFiles parses git status --porcelain output into a list of files
func parseDirtyFiles(statusOutput string) []string {
	if statusOutput == "" {
		return nil
	}

	lines := strings.Split(statusOutput, "\n")
	files := make([]string, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}
		// Keep the exact git status format (don't trim - preserves status codes)
		files = append(files, line)
	}

	return files
}

// parseAheadBehind parses the output of git rev-list --left-right --count
// Expected format: "5\t3" (5 ahead, 3 behind)
// Returns (0, 0) if parsing fails
func parseAheadBehind(output string) (ahead int, behind int) {
	parts := strings.Fields(output)
	if len(parts) != 2 {
		return 0, 0
	}

	ahead, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0
	}

	behind, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0
	}

	return ahead, behind
}
