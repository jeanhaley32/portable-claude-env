package volume

import (
	"fmt"

	"github.com/jeanhaley32/portable-claude-env/internal/constants"
)

// BootstrapConfig holds configuration for creating a new encrypted volume.
type BootstrapConfig struct {
	Path     string
	SizeGB   int
	Password string
}

// Validate checks that the bootstrap configuration is valid.
func (c *BootstrapConfig) Validate() error {
	if c.Path == "" {
		return fmt.Errorf("volume path is required")
	}
	if c.SizeGB < constants.MinVolumeSizeGB || c.SizeGB > constants.MaxVolumeSizeGB {
		return fmt.Errorf("volume size must be between %d and %d GB, got %d",
			constants.MinVolumeSizeGB, constants.MaxVolumeSizeGB, c.SizeGB)
	}
	if c.Password == "" {
		return fmt.Errorf("password is required")
	}
	return nil
}

// VolumeManager handles OS-specific encrypted volume operations.
type VolumeManager interface {
	// Bootstrap creates a new encrypted volume with the given configuration.
	Bootstrap(config BootstrapConfig) error

	// Mount decrypts and mounts the volume, returning the mount point.
	Mount(volumePath, password string) (mountPoint string, err error)

	// Unmount unmounts and closes the encrypted volume.
	Unmount(mountPoint string) error

	// Exists checks if a volume file exists at the given path.
	Exists(volumePath string) bool

	// GetVolumePath returns the expected volume file path for the given base directory.
	GetVolumePath(baseDir string) string

	// IsMounted returns true if the volume is currently mounted.
	IsMounted() bool

	// GetMountPoint returns the current mount point if mounted, empty string otherwise.
	GetMountPoint() string
}
