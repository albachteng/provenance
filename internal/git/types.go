package git

// GitState represents the state of a Git repository at a point in time
type GitState struct {
	// RepoPath is the absolute path to the repository
	RepoPath string

	// Head is the current HEAD commit hash
	Head string

	// Branch is the current branch name
	Branch string

	// IsDirty indicates if there are uncommitted changes
	IsDirty bool

	// DirtyFiles is a list of files with uncommitted changes
	// Format matches git status --short output
	DirtyFiles []string

	// DiffSummary is a human-readable summary of changes
	// Format: "+X -Y" or "X insertions(+), Y deletions(-)"
	DiffSummary string

	// RemoteTracking is the remote tracking branch (e.g., "origin/main")
	RemoteTracking string

	// Ahead is the number of commits ahead of remote
	Ahead int

	// Behind is the number of commits behind remote
	Behind int
}
