package constants

import "os"

// Volume-related constants
const (
	// MacOSVolumeFile is the filename for the encrypted volume on macOS.
	MacOSVolumeFile = "capsule.sparseimage"

	// MacOSVolumeName is the volume label used when creating the encrypted volume.
	MacOSVolumeName = "Capsule"

	// CapsuleConfigDir is the name of the user config directory under home.
	CapsuleConfigDir = ".capsule"

	// VolumesSubdir is the subdirectory under CapsuleConfigDir for volumes.
	VolumesSubdir = "volumes"
)

// Shadow documentation constants
const (
	// DocsSymlinkName is the name of the shadow documentation directory.
	DocsSymlinkName = "_docs"
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
