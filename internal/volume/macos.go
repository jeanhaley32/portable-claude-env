package volume

import (
	"context"
	"crypto/rand"
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
)

// mountPointPrefix is the prefix for unique mount points in /tmp
const mountPointPrefix = "/tmp/capsule-"

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

	volumePath := filepath.Join(cfg.Path, constants.MacOSVolumeFile)

	// Check if volume already exists
	if m.Exists(volumePath) {
		return fmt.Errorf("volume already exists at %s", volumePath)
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
	cmd.Stdin = strings.NewReader(cfg.Password)
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
	if err := m.createDirectoryStructure(mountPoint, cfg.ContextFiles); err != nil {
		// Try to unmount even if directory creation fails
		_ = m.Unmount(mountPoint)
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	// Verify the directory structure was created by reading it back
	homeClaudeDir := filepath.Join(mountPoint, "home", ".claude")
	if _, err := os.ReadDir(homeClaudeDir); err != nil {
		_ = m.Unmount(mountPoint)
		return fmt.Errorf("verification failed - cannot read home/.claude: %w", err)
	}

	// Sync filesystem before unmount
	claudeMDPath := filepath.Join(mountPoint, "home", ".claude", "CLAUDE.md")
	if f, err := os.OpenFile(claudeMDPath, os.O_RDONLY, 0); err == nil {
		if err := f.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: fsync failed: %v\n", err)
		}
		f.Close()
	}

	// Sync the parent directory
	if d, err := os.Open(filepath.Join(mountPoint, "home", ".claude")); err == nil {
		_ = d.Sync()
		d.Close()
	}

	// System-wide sync
	syncCtx, syncCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer syncCancel()
	syncCmd := exec.CommandContext(syncCtx, "/bin/sync")
	if err := syncCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: sync failed: %v\n", err)
	}

	// Additional delay to ensure sparse image is fully written
	time.Sleep(2 * time.Second)

	// Unmount the volume
	if err := m.Unmount(mountPoint); err != nil {
		return fmt.Errorf("failed to unmount volume after setup: %w", err)
	}

	// Wait for unmount to fully complete
	time.Sleep(1 * time.Second)

	// Remount and verify data persisted to the sparse image
	verifyMountPoint, err := m.Mount(volumePath, cfg.Password)
	if err != nil {
		return fmt.Errorf("failed to remount volume for verification: %w", err)
	}

	// Check that CLAUDE.md exists after remount
	claudeMDVerifyPath := filepath.Join(verifyMountPoint, "home", ".claude", "CLAUDE.md")
	if _, err := os.Stat(claudeMDVerifyPath); err != nil {
		_ = m.Unmount(verifyMountPoint)
		return fmt.Errorf("verification failed - CLAUDE.md not persisted to sparse image: %w", err)
	}

	// Final unmount
	if err := m.Unmount(verifyMountPoint); err != nil {
		return fmt.Errorf("failed to unmount volume after verification: %w", err)
	}

	return nil
}

func (m *MacOSVolumeManager) Mount(volumePath, password string) (string, error) {
	// Check if this specific volume is already mounted
	if mountPoint := m.findMountPointForVolume(volumePath); mountPoint != "" {
		return mountPoint, nil
	}

	// Generate a unique mount point in /tmp to avoid Docker Desktop VirtioFS caching issues
	// Each session gets a fresh path that Docker has never seen before
	mountPoint, err := m.generateUniqueMountPoint()
	if err != nil {
		return "", fmt.Errorf("failed to generate mount point: %w", err)
	}

	// Create the mount point directory
	if err := os.MkdirAll(mountPoint, constants.DirPermissions); err != nil {
		return "", fmt.Errorf("failed to create mount point directory: %w", err)
	}

	// Mount with password via stdin to the unique mount point
	ctx, cancel := context.WithTimeout(context.Background(), volumeOperationTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hdiutil", "attach", "-stdinpass", "-mountpoint", mountPoint, volumePath)
	cmd.Stdin = strings.NewReader(password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Clean up the directory we created
		os.Remove(mountPoint)

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

// generateUniqueMountPoint creates a unique path in /tmp for mounting
func (m *MacOSVolumeManager) generateUniqueMountPoint() (string, error) {
	// Generate 8 random bytes for a 16-character hex string
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return mountPointPrefix + hex.EncodeToString(bytes), nil
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
		m.clearVMCache()
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

	// Clear the Linux VM's kernel cache to release VirtioFS file handles
	// This may prevent "operation not permitted" errors on subsequent mounts
	m.clearVMCache()

	return nil
}

// clearVMCache clears the Linux VM's kernel cache to release VirtioFS file handles.
// This drops dentries and inodes which may hold references to unmounted paths.
func (m *MacOSVolumeManager) clearVMCache() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// echo 3 drops page cache, dentries, and inodes
	cmd := exec.CommandContext(ctx, "docker", "run", "--privileged", "--rm",
		"alpine", "sh", "-c", "echo 3 > /proc/sys/vm/drop_caches")
	_ = cmd.Run() // Ignore errors - this is best-effort cache clear
}

func (m *MacOSVolumeManager) Exists(volumePath string) bool {
	_, err := os.Stat(volumePath)
	return err == nil
}

func (m *MacOSVolumeManager) GetVolumePath(baseDir string) string {
	return filepath.Join(baseDir, constants.MacOSVolumeFile)
}

// createDirectoryStructure creates the required directories inside the mounted volume.
func (m *MacOSVolumeManager) createDirectoryStructure(mountPoint string, contextFiles []string) error {
	for _, dir := range config.VolumeStructure {
		path := filepath.Join(mountPoint, dir)
		if err := os.MkdirAll(path, constants.DirPermissions); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Write the bootstrap CLAUDE.md file for Claude Code context
	claudeMDPath := filepath.Join(mountPoint, "home", ".claude", "CLAUDE.md")
	claudeMDContent := embedded.ClaudeMDTemplate
	for _, ctxFile := range contextFiles {
		if extraContent, err := os.ReadFile(ctxFile); err == nil {
			claudeMDContent = claudeMDContent + "\n" + string(extraContent)
		} else {
			return fmt.Errorf("failed to read context file %s: %w", ctxFile, err)
		}
	}
	if err := os.WriteFile(claudeMDPath, []byte(claudeMDContent), constants.FilePermissions); err != nil {
		return fmt.Errorf("failed to write CLAUDE.md: %w", err)
	}

	return nil
}

// findAnyMountedVolume finds the mount point for any mounted capsule volume.
// This is a fallback for cases where we don't know the specific volume path.
func (m *MacOSVolumeManager) findAnyMountedVolume() string {
	// Check for our unique mount points in /tmp
	entries, err := os.ReadDir("/tmp")
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "capsule-") && entry.IsDir() {
			mountPoint := filepath.Join("/tmp", entry.Name())
			// Verify it's actually mounted by checking for content
			contents, err := os.ReadDir(mountPoint)
			if err == nil && len(contents) > 0 {
				return mountPoint
			}
		}
	}

	// Also check legacy /Volumes mount point for backwards compatibility
	if _, err := os.Stat(constants.MacOSMountPoint); err == nil {
		return constants.MacOSMountPoint
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

		// If we found our image, look for mount point
		// Note: hdiutil returns /private/tmp on macOS (since /tmp -> /private/tmp)
		if foundOurImage && (strings.Contains(line, "/tmp/capsule-") || strings.Contains(line, "/private/tmp/capsule-")) {
			// Line format: "/dev/diskXsY	Apple_APFS	/private/tmp/capsule-xxx"
			fields := strings.Fields(line)
			for _, field := range fields {
				if strings.HasPrefix(field, "/tmp/capsule-") || strings.HasPrefix(field, "/private/tmp/capsule-") {
					return field
				}
			}
		}

		// Also check for legacy /Volumes mount point
		if foundOurImage && strings.Contains(line, constants.MacOSMountPoint) {
			fields := strings.Fields(line)
			for _, field := range fields {
				if field == constants.MacOSMountPoint {
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

// IsMounted returns true if the specified volume is currently mounted.
func (m *MacOSVolumeManager) IsMounted(volumePath string) bool {
	return m.findMountPointForVolume(volumePath) != ""
}

// GetMountPoint returns the mount point for the specified volume if mounted, empty string otherwise.
func (m *MacOSVolumeManager) GetMountPoint(volumePath string) string {
	return m.findMountPointForVolume(volumePath)
}
