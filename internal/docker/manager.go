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

// Retry configuration for container readiness
const (
	containerReadyMaxRetries = 10
	containerReadyRetryDelay = 500 * time.Millisecond
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

	// Create and start container with timeout
	// Override entrypoint since Dockerfile uses /bin/bash which doesn't work with tail command
	// Set HOME to encrypted volume so credentials and user data persist
	startTimeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()

	fmt.Fprintf(os.Stderr, "[docker] Starting container with mount: %s\n", config.VolumeMountPoint)

	// Use --mount with consistency=delegated to reduce Docker Desktop caching issues
	// delegated mode gives container authority over filesystem state
	volumeMount := fmt.Sprintf("type=bind,source=%s,target=/claude-env,consistency=delegated", config.VolumeMountPoint)
	workspaceMount := fmt.Sprintf("type=bind,source=%s,target=/workspace,consistency=delegated", config.WorkspacePath)

	cmd := exec.CommandContext(ctx, "docker", "run",
		"-d",
		"--name", config.ContainerName,
		"--mount", volumeMount,
		"--mount", workspaceMount,
		"-w", "/workspace",
		"-e", "HOME=/claude-env/home",
		"--entrypoint", "tail",
		config.ImageName,
		"-f", "/dev/null", // Keep container running
	)

	// Capture stderr to include in error message for retry logic
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("container start timed out after %v", startTimeout)
		}
		return fmt.Errorf("failed to start container: %w: %s", err, strings.TrimSpace(string(output)))
	}

	fmt.Fprintf(os.Stderr, "[docker] Container started successfully\n")
	return nil
}

func (m *Manager) Stop(containerName string) error {
	if containerName == "" {
		containerName = DefaultContainerName
	}

	// Validate container name
	if err := ValidateDockerName(containerName); err != nil {
		return fmt.Errorf("invalid container name: %w", err)
	}

	// Check if container exists
	if !m.containerExists(containerName) {
		return nil // Nothing to stop
	}

	// Stop container with timeout
	if err := m.runCommandWithTimeout(defaultCommandTimeout, "docker", "stop", containerName); err != nil {
		// Try to force stop - log but don't fail if kill also fails
		// The container may have already stopped between the stop and kill commands
		if killErr := m.runCommandWithTimeout(defaultCommandTimeout, "docker", "kill", containerName); killErr != nil {
			// Only return error if container still exists after both attempts
			if m.containerExists(containerName) && m.IsRunning(containerName) {
				return fmt.Errorf("failed to stop container: stop error: %v, kill error: %v", err, killErr)
			}
		}
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

// Exec runs an interactive shell in the container and waits for it to exit.
// This allows cleanup to happen after the user exits the shell.
func (m *Manager) Exec(containerName string) error {
	if containerName == "" {
		containerName = DefaultContainerName
	}

	cmd := exec.Command("docker", "exec", "-it", containerName, "/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run and wait for user to exit
	return cmd.Run()
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
	for i := 0; i < containerReadyMaxRetries; i++ {
		if m.IsRunning(containerName) {
			break
		}
		if i == containerReadyMaxRetries-1 {
			return fmt.Errorf("container %s not running after %d retries", containerName, containerReadyMaxRetries)
		}
		time.Sleep(containerReadyRetryDelay)
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

// CheckTmpFileSharing verifies Docker Desktop can access /tmp for volume mounts.
// This is required because we mount encrypted volumes to unique paths in /tmp.
func (m *Manager) CheckTmpFileSharing() error {
	// Create a test directory with unique name
	testDir := fmt.Sprintf("/tmp/claude-env-docker-check-%d", time.Now().UnixNano())
	if err := os.MkdirAll(testDir, 0755); err != nil {
		return fmt.Errorf("failed to create test directory: %w", err)
	}
	defer os.RemoveAll(testDir)

	// Write a test file
	testFile := testDir + "/test.txt"
	if err := os.WriteFile(testFile, []byte("docker-check"), 0644); err != nil {
		return fmt.Errorf("failed to create test file: %w", err)
	}

	// Try to mount and read from Docker
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", testDir+":/test:ro",
		"alpine", "cat", "/test/test.txt")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf(`Docker cannot access /tmp for file sharing.

Please configure Docker Desktop:
  1. Open Docker Desktop
  2. Go to Settings (gear icon) → Resources → File sharing
  3. Add /tmp (or /private/tmp) to the list
  4. Click "Apply & Restart"

Error: %s`, strings.TrimSpace(string(output)))
	}

	if strings.TrimSpace(string(output)) != "docker-check" {
		return fmt.Errorf("Docker file sharing test returned unexpected output: %s", output)
	}

	return nil
}

// RefreshMountCache forces Docker Desktop to refresh its VirtioFS cache for a mount point.
// This is necessary because Docker Desktop's VirtioFS layer caches mount information,
// and encrypted volumes that appear/disappear can cause stale cache entries.
// By running a container that mounts the specific path, we force VirtioFS to re-scan.
func (m *Manager) RefreshMountCache(mountPoint string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Mount the actual path we'll be using - this forces VirtioFS to refresh its view
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", mountPoint+":/refresh-check:ro",
		"alpine", "ls", "/refresh-check")

	_, err := cmd.CombinedOutput()
	// We don't care about the output, just that Docker accessed the path
	// This refreshes VirtioFS's internal cache for this mount point
	if err != nil {
		// If this fails, the actual mount will likely fail too, but we'll let that
		// error be reported with more context
		return nil
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
