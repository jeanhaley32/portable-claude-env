package volume

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jeanhaley32/claude-capsule/internal/constants"
)

// PathResolver handles volume path resolution with priority rules.
type PathResolver struct {
	homeDir string
}

// NewPathResolver creates a new PathResolver.
func NewPathResolver() (*PathResolver, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	return &PathResolver{homeDir: homeDir}, nil
}

// GetGlobalVolumeDir returns the global volume directory path.
// Returns: ~/.capsule/volumes
func (p *PathResolver) GetGlobalVolumeDir() string {
	return filepath.Join(p.homeDir, constants.CapsuleConfigDir, constants.VolumesSubdir)
}

// GetDefaultVolumePath returns the default global volume path.
// Returns: ~/.capsule/volumes/capsule.sparseimage
func (p *PathResolver) GetDefaultVolumePath() string {
	return filepath.Join(p.GetGlobalVolumeDir(), constants.MacOSVolumeFile)
}

// GetLocalVolumePath returns the local volume path for a given directory.
// Returns: {dir}/capsule.sparseimage
func (p *PathResolver) GetLocalVolumePath(dir string) string {
	return filepath.Join(dir, constants.MacOSVolumeFile)
}

// ResolveVolumePath applies the volume resolution priority rules.
// Priority:
// 1. Explicit path (if provided) - use exactly what user specifies
// 2. Local volume ({cwd}/capsule.sparseimage) - if exists, use it
// 3. Global volume (~/.capsule/volumes/capsule.sparseimage) - default
//
// Returns the resolved volume path and whether it exists.
func (p *PathResolver) ResolveVolumePath(explicitPath, cwd string) (volumePath string, exists bool) {
	// Priority 1: Explicit path
	if explicitPath != "" {
		_, err := os.Stat(explicitPath)
		return explicitPath, err == nil
	}

	// Priority 2: Local volume
	localPath := p.GetLocalVolumePath(cwd)
	if _, err := os.Stat(localPath); err == nil {
		return localPath, true
	}

	// Priority 3: Global volume (default)
	globalPath := p.GetDefaultVolumePath()
	_, err := os.Stat(globalPath)
	return globalPath, err == nil
}

// VolumeNotFoundError provides a helpful error message showing both locations checked.
type VolumeNotFoundError struct {
	LocalPath  string
	GlobalPath string
}

func (e *VolumeNotFoundError) Error() string {
	return fmt.Sprintf("volume not found at:\n  - %s (local)\n  - %s (global)\nRun 'capsule bootstrap' first or specify --volume", e.LocalPath, e.GlobalPath)
}

// ResolveVolumePathStrict is like ResolveVolumePath but returns an error if no volume is found.
func (p *PathResolver) ResolveVolumePathStrict(explicitPath, cwd string) (string, error) {
	volumePath, exists := p.ResolveVolumePath(explicitPath, cwd)
	if !exists {
		return "", &VolumeNotFoundError{
			LocalPath:  p.GetLocalVolumePath(cwd),
			GlobalPath: p.GetDefaultVolumePath(),
		}
	}
	return volumePath, nil
}
