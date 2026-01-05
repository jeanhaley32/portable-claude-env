package docker

import "fmt"

// ContainerConfig holds configuration for starting a container.
type ContainerConfig struct {
	ImageName        string
	ContainerName    string
	VolumeMountPoint string
	WorkspacePath    string
}

// Validate checks that the container configuration is valid.
func (c *ContainerConfig) Validate() error {
	if c.ImageName == "" {
		return fmt.Errorf("image name is required")
	}
	if c.ContainerName == "" {
		return fmt.Errorf("container name is required")
	}
	if c.VolumeMountPoint == "" {
		return fmt.Errorf("volume mount point is required")
	}
	if c.WorkspacePath == "" {
		return fmt.Errorf("workspace path is required")
	}
	return nil
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
