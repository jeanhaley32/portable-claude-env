package symlink

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jeanhaley32/portable-claude-env/internal/constants"
)

// Manager implements SymlinkManager.
type Manager struct{}

// NewManager creates a new symlink manager.
func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) CreateSymlink(workspacePath, volumeMountPoint, repoID string) error {
	// Validate inputs
	if workspacePath == "" || volumeMountPoint == "" || repoID == "" {
		return fmt.Errorf("all parameters are required: workspace=%q, mount=%q, repo=%q",
			workspacePath, volumeMountPoint, repoID)
	}

	symlinkPath := filepath.Join(workspacePath, constants.DocsSymlinkName)
	targetPath := filepath.Join(volumeMountPoint, "repos", repoID)

	// Ensure target directory exists
	if err := os.MkdirAll(targetPath, constants.DirPermissions); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Use atomic symlink replacement to avoid race conditions:
	// 1. Create symlink with temporary name
	// 2. Rename to final name (atomic on Unix)
	tempPath := symlinkPath + ".tmp"

	// Remove any stale temp symlink
	_ = os.Remove(tempPath)

	// Create temp symlink
	if err := os.Symlink(targetPath, tempPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	// Atomic rename to final path
	if err := os.Rename(tempPath, symlinkPath); err != nil {
		// Clean up temp symlink on failure
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename symlink: %w", err)
	}

	return nil
}

func (m *Manager) RemoveSymlink(workspacePath string) error {
	symlinkPath := filepath.Join(workspacePath, constants.DocsSymlinkName)

	// Only remove if it's a symlink, not a real directory
	info, err := os.Lstat(symlinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(symlinkPath)
	}

	return fmt.Errorf("%s exists but is not a symlink", symlinkPath)
}

func (m *Manager) SymlinkExists(workspacePath string) bool {
	symlinkPath := filepath.Join(workspacePath, constants.DocsSymlinkName)
	info, err := os.Lstat(symlinkPath)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

func (m *Manager) CleanupBroken(workspacePath string) error {
	symlinkPath := filepath.Join(workspacePath, constants.DocsSymlinkName)

	// Check if symlink exists
	info, err := os.Lstat(symlinkPath)
	if err != nil {
		return nil // Doesn't exist, nothing to clean
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return nil // Not a symlink
	}

	// Check if target exists
	_, err = os.Stat(symlinkPath)
	if os.IsNotExist(err) {
		// Broken symlink, remove it
		return os.Remove(symlinkPath)
	}

	return nil
}
