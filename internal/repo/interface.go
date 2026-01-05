package repo

// Identifier generates consistent repository IDs.
type Identifier interface {
	// GetRepoID returns a unique, filesystem-safe identifier for the repository.
	// For git repos, this is derived from the remote URL.
	// For non-git directories, this is derived from the directory name.
	GetRepoID(workspacePath string) (string, error)

	// GetWorkspaceRoot returns the root directory of the workspace.
	// For git repos, this is the git root.
	// For non-git directories, this is the provided path.
	GetWorkspaceRoot(path string) (string, error)
}
