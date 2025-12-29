package git

// Provider is an interface for Git state capture implementations
// This allows swapping between os/exec-based and go-git library implementations
type Provider interface {
	// CaptureState captures the current state of a Git repository
	CaptureState(repoPath string) (*GitState, error)
}

// defaultProvider is the package-level provider instance
var defaultProvider Provider

// SetProvider sets the global Git provider implementation
// This allows tests or applications to swap implementations
func SetProvider(p Provider) {
	defaultProvider = p
}

// GetProvider returns the current Git provider
// If no provider is set, returns the default CLI-based provider
func GetProvider() Provider {
	if defaultProvider == nil {
		defaultProvider = &CLIProvider{}
	}
	return defaultProvider
}

// CaptureGitState captures Git repository state using the configured provider
// This is the main entry point for Git state capture
func CaptureGitState(repoPath string) (*GitState, error) {
	return GetProvider().CaptureState(repoPath)
}
