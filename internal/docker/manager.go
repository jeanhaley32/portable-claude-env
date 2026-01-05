package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Default timeout for Docker commands
const defaultCommandTimeout = 30 * time.Second

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
	// Validate configuration
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid container config: %w", err)
	}

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
	// Override entrypoint since Dockerfile uses /bin/bash which doesn't work with tail command
	// Set HOME to encrypted volume so credentials and user data persist
	cmd := exec.Command("docker", "run",
		"-d",
		"--name", config.ContainerName,
		"-v", config.VolumeMountPoint+":/claude-env",
		"-v", config.WorkspacePath+":/workspace",
		"-w", "/workspace",
		"-e", "HOME=/claude-env/home",
		"--entrypoint", "tail",
		config.ImageName,
		"-f", "/dev/null", // Keep container running
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

// SetupWorkspaceSymlink creates the _docs symlink inside the container.
// It waits for the container to be ready and then runs the setup script.
func (m *Manager) SetupWorkspaceSymlink(containerName, repoID string) error {
	if containerName == "" {
		containerName = DefaultContainerName
	}
	if repoID == "" {
		return fmt.Errorf("repoID is required")
	}

	// Wait for container to be running with retry
	maxRetries := 10
	retryDelay := 500 * time.Millisecond
	for i := 0; i < maxRetries; i++ {
		if m.IsRunning(containerName) {
			break
		}
		if i == maxRetries-1 {
			return fmt.Errorf("container %s not running after %d retries", containerName, maxRetries)
		}
		time.Sleep(retryDelay)
	}

	// Run the setup script inside the container
	ctx, cancel := context.WithTimeout(context.Background(), defaultCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"setup-workspace-symlink.sh", repoID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("symlink setup timed out after %v", defaultCommandTimeout)
		}
		return fmt.Errorf("failed to setup workspace symlink: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// runCommandWithTimeout runs a command with a timeout.
func (m *Manager) runCommandWithTimeout(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command timed out after %v", timeout)
	}
	return err
}

// getCommandOutputWithTimeout runs a command and returns output with a timeout.
func (m *Manager) getCommandOutputWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("command timed out after %v", timeout)
	}
	return output, err
}

// checkDockerRunning verifies Docker daemon is running.
func (m *Manager) checkDockerRunning() error {
	if err := m.runCommandWithTimeout(defaultCommandTimeout, "docker", "info"); err != nil {
		return fmt.Errorf("Docker is not running. Please start Docker Desktop: %w", err)
	}
	return nil
}

// imageExists checks if a Docker image exists locally.
func (m *Manager) imageExists(imageName string) bool {
	return m.runCommandWithTimeout(defaultCommandTimeout, "docker", "image", "inspect", imageName) == nil
}

// containerExists checks if a container exists (running or stopped).
func (m *Manager) containerExists(containerName string) bool {
	output, err := m.getCommandOutputWithTimeout(defaultCommandTimeout, "docker", "ps", "-a", "-q", "-f", "name=^"+containerName+"$")
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// removeContainer removes a container.
func (m *Manager) removeContainer(containerName string) error {
	return m.runCommandWithTimeout(defaultCommandTimeout, "docker", "rm", "-f", containerName)
}
