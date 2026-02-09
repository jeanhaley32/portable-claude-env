package volume

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeanhaley32/claude-capsule/internal/config"
	"github.com/jeanhaley32/claude-capsule/internal/constants"
	"github.com/jeanhaley32/claude-capsule/internal/embedded"
	"github.com/jeanhaley32/claude-capsule/internal/terminal"
)

// mountPointPrefix is the prefix for mount points in /Volumes (standard macOS location)
const mountPointPrefix = "/Volumes/Capsule-"

// Timeout for volume operations (hdiutil can be slow for large volumes)
const volumeOperationTimeout = 5 * time.Minute

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

	volumePath := cfg.VolumePath

	// Check if volume already exists
	if m.Exists(volumePath) {
		return fmt.Errorf("volume already exists at %s", volumePath)
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(volumePath)
	if err := os.MkdirAll(parentDir, constants.DirPermissions); err != nil {
		return fmt.Errorf("failed to create parent directory %s: %w", parentDir, err)
	}

	// Create encrypted sparse image with timeout
	// hdiutil create -size <size>g -encryption AES-256 -type SPARSE -fs APFS -volname ClaudeEnv -stdinpass <path>
	ctx, cancel := context.WithTimeout(context.Background(), volumeOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hdiutil", "create",
		"-size", fmt.Sprintf("%dg", cfg.SizeGB),
		"-encryption", "AES-256",
		"-type", "SPARSE",
		"-fs", "APFS",
		"-volname", constants.MacOSVolumeName,
		"-stdinpass",
		volumePath,
	)
	cmd.Stdin = cfg.Password.Reader()
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("volume creation timed out after %v", volumeOperationTimeout)
		}
		return fmt.Errorf("failed to create encrypted volume: %w", err)
	}

	// Mount the new volume to create directory structure
	mountPoint, err := m.Mount(volumePath, cfg.Password)
	if err != nil {
		return fmt.Errorf("failed to mount new volume: %w", err)
	}

	// Create directory structure
	if err := m.createDirectoryStructure(mountPoint, cfg); err != nil {
		// Try to unmount even if directory creation fails
		_ = m.Unmount(mountPoint)
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	// Unmount the volume - APFS handles durability, unmount syncs data
	if err := m.Unmount(mountPoint); err != nil {
		return fmt.Errorf("failed to unmount volume after setup: %w", err)
	}

	return nil
}

func (m *MacOSVolumeManager) Mount(volumePath string, password *terminal.SecurePassword) (string, error) {
	// Check if this specific volume is already mounted
	if mountPoint := m.findMountPointForVolume(volumePath); mountPoint != "" {
		return mountPoint, nil
	}

	// Generate a deterministic mount point in /Volumes based on the volume path
	// Using /Volumes is the standard macOS location and works reliably with Docker Desktop
	mountPoint := m.generateMountPoint(volumePath)

	// Mount with password via stdin
	// hdiutil will create the mount point in /Volumes (it has system entitlements to do so)
	ctx, cancel := context.WithTimeout(context.Background(), volumeOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hdiutil", "attach", "-stdinpass", "-mountpoint", mountPoint, volumePath)
	cmd.Stdin = password.Reader()

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("volume mount timed out after %v", volumeOperationTimeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to mount volume: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to mount volume: %w: %s", err, string(output))
	}

	return mountPoint, nil
}

// generateMountPoint creates a deterministic mount point path based on the volume file path.
// This ensures the same volume always mounts to the same location, which works better
// with Docker Desktop's VirtioFS caching.
func (m *MacOSVolumeManager) generateMountPoint(volumePath string) string {
	// Hash the volume path to get a deterministic, short identifier
	hash := sha256.Sum256([]byte(volumePath))
	shortHash := hex.EncodeToString(hash[:])[:12]
	return mountPointPrefix + shortHash
}

func (m *MacOSVolumeManager) Unmount(mountPoint string) error {
	if mountPoint == "" {
		// If no mount point specified, try to find any mounted claude-env volume
		mountPoint = m.findAnyMountedVolume()
		if mountPoint == "" {
			return nil // Not mounted, nothing to do
		}
	}

	// Use shorter timeout for unmount operations
	unmountTimeout := 30 * time.Second

	// Try diskutil unmount first (cleaner, forces sync)
	diskutilCtx, diskutilCancel := context.WithTimeout(context.Background(), unmountTimeout)
	defer diskutilCancel()

	diskutilCmd := exec.CommandContext(diskutilCtx, "diskutil", "unmount", mountPoint)
	if err := diskutilCmd.Run(); err == nil {
		// diskutil unmount succeeded, clean up mount point directory
		if strings.HasPrefix(mountPoint, mountPointPrefix) {
			os.Remove(mountPoint)
		}
		return nil
	}

	// Fall back to hdiutil detach
	ctx, cancel := context.WithTimeout(context.Background(), unmountTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hdiutil", "detach", mountPoint)
	if err := cmd.Run(); err != nil {
		// Try force detach with fresh context
		forceCtx, forceCancel := context.WithTimeout(context.Background(), unmountTimeout)
		defer forceCancel()

		cmd = exec.CommandContext(forceCtx, "hdiutil", "detach", "-force", mountPoint)
		if err := cmd.Run(); err != nil {
			if forceCtx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("volume unmount timed out after %v (even with force)", unmountTimeout)
			}
			return fmt.Errorf("failed to unmount volume (even with force): %w", err)
		}
	}

	// Clean up our mount point directory in /tmp
	// Only remove if it's one of our managed mount points (safety check)
	if strings.HasPrefix(mountPoint, mountPointPrefix) {
		os.Remove(mountPoint)
	}

	return nil
}

func (m *MacOSVolumeManager) Exists(volumePath string) bool {
	_, err := os.Stat(volumePath)
	return err == nil
}

// createDirectoryStructure creates the required directories inside the mounted volume.
func (m *MacOSVolumeManager) createDirectoryStructure(mountPoint string, cfg BootstrapConfig) error {
	for _, dir := range config.VolumeStructure {
		path := filepath.Join(mountPoint, dir)
		if err := os.MkdirAll(path, constants.DirPermissions); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Build CLAUDE.md content
	claudeMDContent := embedded.ClaudeMDTemplate

	// Append context files
	for _, ctxFile := range cfg.ContextFiles {
		if extraContent, err := os.ReadFile(ctxFile); err == nil {
			claudeMDContent = claudeMDContent + "\n" + string(extraContent)
		} else {
			return fmt.Errorf("failed to read context file %s: %w", ctxFile, err)
		}
	}

	// Append memory protocol docs
	claudeMDContent = claudeMDContent + embedded.MemoryProtocolDocs
	claudeMDContent = claudeMDContent + embedded.BeadsProtocolDocs

	// Write CLAUDE.md
	claudeMDPath := filepath.Join(mountPoint, "home", ".claude", "CLAUDE.md")
	if err := os.WriteFile(claudeMDPath, []byte(claudeMDContent), constants.FilePermissions); err != nil {
		return fmt.Errorf("failed to write CLAUDE.md: %w", err)
	}

	// Install doc-sync skill and memory system
	if err := embedded.WriteDocSyncFiles(mountPoint); err != nil {
		return fmt.Errorf("failed to install doc-sync: %w", err)
	}
	if err := embedded.WriteSettingsJSON(mountPoint); err != nil {
		return fmt.Errorf(`failed to write settings.json: %w

Recovery: Manually add to ~/.claude/settings.json inside the container:
  "mcpServers": { "doc-sync": { "command": "python3", "args": ["/claude-env/home/.claude/skills/doc-sync/mcp_server.py"] } }
Or delete the volume and re-run bootstrap.`, err)
	}
	if cfg.Version != "" {
		if err := embedded.WriteVersionFile(mountPoint, cfg.Version); err != nil {
			return fmt.Errorf("failed to write VERSION: %w", err)
		}
	}

	return nil
}

// findAnyMountedVolume finds the mount point for any mounted capsule volume.
// This is a fallback for cases where we don't know the specific volume path.
func (m *MacOSVolumeManager) findAnyMountedVolume() string {
	// Check for our mount points in /Volumes
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "Capsule-") && entry.IsDir() {
			mountPoint := filepath.Join("/Volumes", entry.Name())
			// Verify it's actually mounted by checking for content
			contents, err := os.ReadDir(mountPoint)
			if err == nil && len(contents) > 0 {
				return mountPoint
			}
		}
	}

	return ""
}

// findMountPointForVolume uses hdiutil info to find the mount point for a specific volume file.
func (m *MacOSVolumeManager) findMountPointForVolume(volumePath string) string {
	if volumePath == "" {
		return ""
	}

	// Resolve to absolute path for comparison
	absVolumePath, err := filepath.Abs(volumePath)
	if err != nil {
		return ""
	}

	// Use hdiutil info to get information about mounted disk images
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hdiutil", "info")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse hdiutil info output to find our volume
	// Format is blocks of info separated by "================================================"
	// Each block has "image-path : /path/to/image" and mount points
	lines := strings.Split(string(output), "\n")
	var currentImagePath string
	var foundOurImage bool

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for image path
		if strings.HasPrefix(line, "image-path") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				currentImagePath = strings.TrimSpace(parts[1])
				// Check if this is our volume (compare absolute paths)
				absCurrentPath, err := filepath.Abs(currentImagePath)
				if err == nil {
					foundOurImage = absCurrentPath == absVolumePath
				}
			}
		}

		// If we found our image, look for mount point in /Volumes
		if foundOurImage && strings.Contains(line, "/Volumes/Capsule-") {
			// Line format: "/dev/diskXsY	Apple_APFS	/Volumes/Capsule-xxx"
			fields := strings.Fields(line)
			for _, field := range fields {
				if strings.HasPrefix(field, "/Volumes/Capsule-") {
					return field
				}
			}
		}

		// Reset on separator
		if strings.HasPrefix(line, "===") {
			foundOurImage = false
			currentImagePath = ""
		}
	}

	return ""
}

// GetMountPoint returns the mount point for the specified volume if mounted, empty string otherwise.
func (m *MacOSVolumeManager) GetMountPoint(volumePath string) string {
	return m.findMountPointForVolume(volumePath)
}
