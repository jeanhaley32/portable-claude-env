package symlink

import (
	"fmt"
	"os"
	"path/filepath"
)

const docsDir = "_docs"

// Manager implements SymlinkManager.
type Manager struct{}

// NewManager creates a new symlink manager.
func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) CreateSymlink(workspacePath, volumeMountPoint, repoID string) error {
	symlinkPath := filepath.Join(workspacePath, docsDir)
	targetPath := filepath.Join(volumeMountPoint, "repos", repoID)

	// Ensure target directory exists
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Remove existing symlink if present
	if m.SymlinkExists(workspacePath) {
		if err := m.RemoveSymlink(workspacePath); err != nil {
			return fmt.Errorf("failed to remove existing symlink: %w", err)
		}
	}

	// Create symlink
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

func (m *Manager) RemoveSymlink(workspacePath string) error {
	symlinkPath := filepath.Join(workspacePath, docsDir)

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
	symlinkPath := filepath.Join(workspacePath, docsDir)
	info, err := os.Lstat(symlinkPath)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

func (m *Manager) CleanupBroken(workspacePath string) error {
	symlinkPath := filepath.Join(workspacePath, docsDir)

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
