package docker

// ContainerConfig holds configuration for starting a container.
type ContainerConfig struct {
	ImageName      string
	ContainerName  string
	VolumeMountPoint string
	WorkspacePath  string
}

// DockerManager handles container operations.
type DockerManager interface {
	// Start creates and starts a container with the given configuration.
	Start(config ContainerConfig) error

	// Stop stops and removes the container.
	Stop(containerName string) error

	// IsRunning checks if a container with the given name is running.
	IsRunning(containerName string) bool
}
