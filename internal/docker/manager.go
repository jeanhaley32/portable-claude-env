package docker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	DefaultImageName     = "portable-claude:latest"
	DefaultContainerName = "portable-claude"
)

// Manager implements DockerManager using the Docker CLI.
type Manager struct{}

// NewManager creates a new Docker manager.
func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Start(config ContainerConfig) error {
	// Check if Docker is running
	if err := m.checkDockerRunning(); err != nil {
		return err
	}

	// Check if image exists
	if !m.imageExists(config.ImageName) {
		return fmt.Errorf("docker image '%s' not found. Build it with: docker build -t %s .",
			config.ImageName, config.ImageName)
	}

	// Check if container already exists
	if m.containerExists(config.ContainerName) {
		if m.IsRunning(config.ContainerName) {
			// Already running, nothing to do
			return nil
		}
		// Exists but not running, remove it
		if err := m.removeContainer(config.ContainerName); err != nil {
			return fmt.Errorf("failed to remove existing container: %w", err)
		}
	}

	// Create and start container
	// docker run -d --name <name> -v <volume>:/claude-env -v <workspace>:/workspace -it <image> tail -f /dev/null
	cmd := exec.Command("docker", "run",
		"-d",
		"--name", config.ContainerName,
		"-v", config.VolumeMountPoint+":/claude-env",
		"-v", config.WorkspacePath+":/workspace",
		"-w", "/workspace",
		"-it",
		config.ImageName,
		"tail", "-f", "/dev/null", // Keep container running
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

func (m *Manager) Stop(containerName string) error {
	if containerName == "" {
		containerName = DefaultContainerName
	}

	// Check if container exists
	if !m.containerExists(containerName) {
		return nil // Nothing to stop
	}

	// Stop container
	cmd := exec.Command("docker", "stop", containerName)
	if err := cmd.Run(); err != nil {
		// Try to force stop
		cmd = exec.Command("docker", "kill", containerName)
		_ = cmd.Run()
	}

	// Remove container
	return m.removeContainer(containerName)
}

func (m *Manager) IsRunning(containerName string) bool {
	if containerName == "" {
		containerName = DefaultContainerName
	}

	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerName)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(output)) == "true"
}

// Exec replaces the current process with docker exec into the container.
// This is used for interactive sessions.
func (m *Manager) Exec(containerName string) error {
	if containerName == "" {
		containerName = DefaultContainerName
	}

	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker not found in PATH: %w", err)
	}

	// Replace current process with docker exec
	// Note: This function does not return on success
	args := []string{"docker", "exec", "-it", containerName, "/bin/bash"}
	env := os.Environ()

	return execSyscall(dockerPath, args, env)
}

// ExecCommand runs a command in the container and returns the output.
func (m *Manager) ExecCommand(containerName string, command ...string) (string, error) {
	if containerName == "" {
		containerName = DefaultContainerName
	}

	args := append([]string{"exec", containerName}, command...)
	cmd := exec.Command("docker", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to exec command in container: %w", err)
	}

	return string(output), nil
}

// checkDockerRunning verifies Docker daemon is running.
func (m *Manager) checkDockerRunning() error {
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Docker is not running. Please start Docker Desktop")
	}
	return nil
}

// imageExists checks if a Docker image exists locally.
func (m *Manager) imageExists(imageName string) bool {
	cmd := exec.Command("docker", "image", "inspect", imageName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// containerExists checks if a container exists (running or stopped).
func (m *Manager) containerExists(containerName string) bool {
	cmd := exec.Command("docker", "ps", "-a", "-q", "-f", "name=^"+containerName+"$")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// removeContainer removes a container.
func (m *Manager) removeContainer(containerName string) error {
	cmd := exec.Command("docker", "rm", "-f", containerName)
	return cmd.Run()
}
