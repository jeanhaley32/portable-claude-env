package volume

// BootstrapConfig holds configuration for creating a new encrypted volume.
type BootstrapConfig struct {
	Path     string
	SizeGB   int
	Password string
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
}
