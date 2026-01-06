package constants

import "os"

// Volume-related constants
const (
	// MacOSVolumeFile is the filename for the encrypted volume on macOS.
	MacOSVolumeFile = "claude-env.sparseimage"

	// MacOSMountPoint is the default mount point for the encrypted volume on macOS.
	MacOSMountPoint = "/Volumes/ClaudeEnv"

	// MacOSVolumeName is the volume label used when creating the encrypted volume.
	MacOSVolumeName = "ClaudeEnv"

	// LinuxVolumeFile is the filename for the encrypted volume on Linux.
	LinuxVolumeFile = "claude-env.img"

	// LinuxMountPoint is the default mount point for the encrypted volume on Linux.
	LinuxMountPoint = "/tmp/claude-env-mount"

	// LinuxMapperName is the device mapper name used for LUKS volumes.
	LinuxMapperName = "claude-env"
)

// Docker-related constants
const (
	// DefaultImageName is the default Docker image name.
	DefaultImageName = "portable-claude:latest"

	// DefaultContainerName is the default Docker container name.
	DefaultContainerName = "portable-claude"
)

// Shadow documentation constants
const (
	// DocsSymlinkName is the name of the shadow documentation directory.
	DocsSymlinkName = "_docs"
)

// Security-related constants
const (
	// MinPasswordLength is the minimum required password length.
	// NIST SP 800-63B recommends minimum 8, but 12+ is better for encryption keys.
	MinPasswordLength = 12
)

// Volume size limits
const (
	// MinVolumeSizeGB is the minimum volume size in gigabytes.
	MinVolumeSizeGB = 1
	// MaxVolumeSizeGB is the maximum volume size in gigabytes.
	MaxVolumeSizeGB = 100
)

// File permissions
const (
	// DirPermissions is the default permission mode for directories.
	DirPermissions os.FileMode = 0755

	// FilePermissions is the default permission mode for sensitive files.
	FilePermissions os.FileMode = 0600
)
