package git

import (
	"fmt"
	"strings"
)

// LineBlame contains blame information for a single line
type LineBlame struct {
	Commit  string
	Content string
	Line    int
}

// BlameLines returns commit info for each line in a range
// Uses git blame --porcelain format for accurate parsing
func BlameLines(repoPath, filePath string, startLine, endLine int) ([]LineBlame, error) {
	if !isGitRepo(repoPath) {
		return nil, fmt.Errorf("not a git repository: %s", repoPath)
	}

	if startLine < 1 || endLine < startLine {
		return nil, fmt.Errorf("invalid line range: %d-%d", startLine, endLine)
	}

	// Use --porcelain for machine-readable output
	lineRange := fmt.Sprintf("%d,%d", startLine, endLine)
	output, err := runGitCommand(repoPath, "blame", "-L", lineRange, "--porcelain", filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to run git blame: %w", err)
	}

	return parseBlameOutput(output, startLine), nil
}

// parseBlameOutput parses git blame --porcelain format
// Format:
//   <sha> <original-line> <final-line> <num-lines>
//   author <author-name>
//   author-time <timestamp>
//   ...
//   \t<line-content>
func parseBlameOutput(output string, startLine int) []LineBlame {
	lines := []LineBlame{}
	currentCommit := ""
	currentLineNum := startLine

	outputLines := strings.Split(output, "\n")

	for _, line := range outputLines {
		if line == "" {
			continue
		}

		// Lines starting with commit SHA (40 hex chars)
		if !strings.HasPrefix(line, "\t") {
			fields := strings.Fields(line)
			if len(fields) > 0 && len(fields[0]) == 40 {
				currentCommit = fields[0]
			}
			continue
		}

		// Actual code lines (prefixed with tab)
		if strings.HasPrefix(line, "\t") && currentCommit != "" {
			lines = append(lines, LineBlame{
				Commit:  currentCommit,
				Content: strings.TrimPrefix(line, "\t"),
				Line:    currentLineNum,
			})
			currentLineNum++
		}
	}

	return lines
}

// GetCommitsForLines returns unique commits that last modified the given line range
func GetCommitsForLines(repoPath, filePath string, startLine, endLine int) ([]string, error) {
	blameLines, err := BlameLines(repoPath, filePath, startLine, endLine)
	if err != nil {
		return nil, err
	}

	// Deduplicate commits while preserving order
	seen := make(map[string]bool)
	commits := []string{}

	for _, blame := range blameLines {
		if !seen[blame.Commit] {
			seen[blame.Commit] = true
			commits = append(commits, blame.Commit)
		}
	}

	return commits, nil
}
