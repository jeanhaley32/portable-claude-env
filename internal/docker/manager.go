package docker

import (
	"fmt"
)

const (
	DefaultImageName     = "portable-claude:latest"
	DefaultContainerName = "portable-claude"
)

// Manager implements DockerManager using the Docker SDK.
type Manager struct{}

// NewManager creates a new Docker manager.
func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Start(config ContainerConfig) error {
	// TODO: Implement using Docker SDK
	// 1. Pull image if not present
	// 2. Create container with mounts:
	//    - config.VolumeMountPoint -> /claude-env (rw)
	//    - config.WorkspacePath -> /workspace (rw)
	// 3. Start container
	// 4. Attach to container (interactive)
	return fmt.Errorf("docker start not yet implemented")
}

func (m *Manager) Stop(containerName string) error {
	// TODO: Implement using Docker SDK
	// 1. Stop container
	// 2. Remove container
	return fmt.Errorf("docker stop not yet implemented")
}

func (m *Manager) IsRunning(containerName string) bool {
	// TODO: Implement using Docker SDK
	// Check if container exists and is running
	return false
}
