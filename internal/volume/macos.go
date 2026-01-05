package volume

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jeanhaley32/portable-claude-env/internal/config"
	"github.com/jeanhaley32/portable-claude-env/internal/constants"
)

// MacOSVolumeManager implements VolumeManager using hdiutil for macOS.
type MacOSVolumeManager struct{}

// NewMacOSVolumeManager creates a new macOS volume manager.
func NewMacOSVolumeManager() *MacOSVolumeManager {
	return &MacOSVolumeManager{}
}

func (m *MacOSVolumeManager) Bootstrap(cfg BootstrapConfig) error {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid bootstrap config: %w", err)
	}

	volumePath := filepath.Join(cfg.Path, constants.MacOSVolumeFile)

	// Check if volume already exists
	if m.Exists(volumePath) {
		return fmt.Errorf("volume already exists at %s", volumePath)
	}

	// Create encrypted sparse image
	// hdiutil create -size <size>g -encryption AES-256 -type SPARSE -fs APFS -volname ClaudeEnv -stdinpass <path>
	cmd := exec.Command("hdiutil", "create",
		"-size", fmt.Sprintf("%dg", cfg.SizeGB),
		"-encryption", "AES-256",
		"-type", "SPARSE",
		"-fs", "APFS",
		"-volname", constants.MacOSVolumeName,
		"-stdinpass",
		volumePath,
	)
	cmd.Stdin = strings.NewReader(cfg.Password)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create encrypted volume: %w", err)
	}

	// Mount the new volume to create directory structure
	mountPoint, err := m.Mount(volumePath, cfg.Password)
	if err != nil {
		return fmt.Errorf("failed to mount new volume: %w", err)
	}

	// Create directory structure
	if err := m.createDirectoryStructure(mountPoint); err != nil {
		// Try to unmount even if directory creation fails
		_ = m.Unmount(mountPoint)
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	// Unmount the volume
	if err := m.Unmount(mountPoint); err != nil {
		return fmt.Errorf("failed to unmount volume after setup: %w", err)
	}

	return nil
}

func (m *MacOSVolumeManager) Mount(volumePath, password string) (string, error) {
	// Check if already mounted
	if mountPoint := m.findMountPoint(); mountPoint != "" {
		return mountPoint, nil
	}

	// Mount with password via stdin
	cmd := exec.Command("hdiutil", "attach", "-stdinpass", volumePath)
	cmd.Stdin = strings.NewReader(password)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to mount volume: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to mount volume: %w", err)
	}

	// Parse output to find mount point
	// Output format: /dev/disk4s1    Apple_APFS                      /Volumes/ClaudeEnv
	mountPoint := m.parseMountPoint(string(output))
	if mountPoint == "" {
		return "", fmt.Errorf("volume mounted but could not determine mount point")
	}

	return mountPoint, nil
}

func (m *MacOSVolumeManager) Unmount(mountPoint string) error {
	if mountPoint == "" {
		mountPoint = m.findMountPoint()
		if mountPoint == "" {
			return nil // Not mounted, nothing to do
		}
	}

	cmd := exec.Command("hdiutil", "detach", mountPoint)
	if err := cmd.Run(); err != nil {
		// Try force detach
		cmd = exec.Command("hdiutil", "detach", "-force", mountPoint)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to unmount volume (even with force): %w", err)
		}
	}

	return nil
}

func (m *MacOSVolumeManager) Exists(volumePath string) bool {
	_, err := os.Stat(volumePath)
	return err == nil
}

func (m *MacOSVolumeManager) GetVolumePath(baseDir string) string {
	return filepath.Join(baseDir, constants.MacOSVolumeFile)
}

// createDirectoryStructure creates the required directories inside the mounted volume.
func (m *MacOSVolumeManager) createDirectoryStructure(mountPoint string) error {
	for _, dir := range config.VolumeStructure {
		path := filepath.Join(mountPoint, dir)
		if err := os.MkdirAll(path, constants.DirPermissions); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// parseMountPoint extracts the mount point from hdiutil attach output.
func (m *MacOSVolumeManager) parseMountPoint(output string) string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		// Look for /Volumes/ in the line
		if idx := strings.Index(line, "/Volumes/"); idx != -1 {
			return strings.TrimSpace(line[idx:])
		}
	}
	return ""
}

// findMountPoint checks if ClaudeEnv volume is currently mounted.
func (m *MacOSVolumeManager) findMountPoint() string {
	// Check standard mount point
	if _, err := os.Stat(constants.MacOSMountPoint); err == nil {
		return constants.MacOSMountPoint
	}

	// Check for numbered variants (e.g., /Volumes/ClaudeEnv 1)
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), constants.MacOSVolumeName) {
			return filepath.Join("/Volumes", entry.Name())
		}
	}

	return ""
}

// IsMounted returns true if the volume is currently mounted.
func (m *MacOSVolumeManager) IsMounted() bool {
	return m.findMountPoint() != ""
}

// GetMountPoint returns the current mount point if mounted, empty string otherwise.
func (m *MacOSVolumeManager) GetMountPoint() string {
	return m.findMountPoint()
}
