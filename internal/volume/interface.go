package volume

import (
	"fmt"

	"github.com/jeanhaley32/claude-capsule/internal/constants"
	"github.com/jeanhaley32/claude-capsule/internal/terminal"
)

// BootstrapConfig holds configuration for creating a new encrypted volume.
type BootstrapConfig struct {
	VolumePath   string // Full path to the volume file (not just directory)
	SizeGB       int
	Password     *terminal.SecurePassword
	ContextFiles []string // Markdown files to extend Claude context
	WithMemory   bool     // Install doc-sync skill with SQLite memory system
	Version      string   // Capsule version for tracking installed components
}

// Validate checks that the bootstrap configuration is valid.
func (c *BootstrapConfig) Validate() error {
	if c.VolumePath == "" {
		return fmt.Errorf("volume path is required")
	}
	if c.SizeGB < constants.MinVolumeSizeGB || c.SizeGB > constants.MaxVolumeSizeGB {
		return fmt.Errorf("volume size must be between %d and %d GB, got %d",
			constants.MinVolumeSizeGB, constants.MaxVolumeSizeGB, c.SizeGB)
	}
	if c.Password == nil || c.Password.Len() == 0 {
		return fmt.Errorf("password is required")
	}
	return nil
}

// VolumeManager handles OS-specific encrypted volume operations.
type VolumeManager interface {
	// Bootstrap creates a new encrypted volume with the given configuration.
	Bootstrap(config BootstrapConfig) error

	// Mount decrypts and mounts the volume, returning the mount point.
	// The caller should clear the password after Mount returns.
	Mount(volumePath string, password *terminal.SecurePassword) (mountPoint string, err error)

	// Unmount unmounts and closes the encrypted volume.
	Unmount(mountPoint string) error

	// Exists checks if a volume file exists at the given path.
	Exists(volumePath string) bool

	// GetMountPoint returns the mount point for the specified volume if mounted, empty string otherwise.
	GetMountPoint(volumePath string) string
}
