package symlink

// SymlinkManager handles _docs symlink creation and cleanup.
type SymlinkManager interface {
	// CreateSymlink creates a symlink from workspace/_docs to the repo's directory in the encrypted volume.
	CreateSymlink(workspacePath, volumeMountPoint, repoID string) error

	// RemoveSymlink removes the _docs symlink from the workspace.
	RemoveSymlink(workspacePath string) error

	// SymlinkExists checks if a _docs symlink exists in the workspace.
	SymlinkExists(workspacePath string) bool

	// CleanupBroken removes the symlink if it's broken (target doesn't exist).
	CleanupBroken(workspacePath string) error
}
