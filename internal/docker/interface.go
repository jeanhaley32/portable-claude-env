package docker

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// validDockerNamePattern validates Docker container and image names.
// Names must start with alphanumeric and contain only alphanumeric, underscore, period, or hyphen.
// Image names may also contain a tag suffix (e.g., "image:tag").
var validDockerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ValidateDockerName checks if a name is valid for Docker container/image.
func ValidateDockerName(name string) error {
	if name == "" {
		return fmt.Errorf("docker name cannot be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("docker name too long: %d characters (max 128)", len(name))
	}
	// For image names, strip the tag for validation
	baseName := name
	if idx := strings.LastIndex(name, ":"); idx > 0 {
		baseName = name[:idx]
	}
	if !validDockerNamePattern.MatchString(baseName) {
		return fmt.Errorf("invalid docker name %q: must start with alphanumeric and contain only [a-zA-Z0-9_.-]", name)
	}
	return nil
}

// validatePath checks for path traversal attacks and validates the path is reasonable.
func validatePath(path, fieldName string) error {
	if path == "" {
		return fmt.Errorf("%s is required", fieldName)
	}
	// Check for path traversal attempts
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("%s contains path traversal: %q", fieldName, path)
	}
	// Require absolute paths
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s must be an absolute path: %q", fieldName, path)
	}
	return nil
}

// ContainerConfig holds configuration for starting a container.
type ContainerConfig struct {
	ImageName        string
	ContainerName    string
	VolumeMountPoint string
	WorkspacePath    string
}

// Validate checks that the container configuration is valid.
func (c *ContainerConfig) Validate() error {
	// Validate image name
	if err := ValidateDockerName(c.ImageName); err != nil {
		return fmt.Errorf("invalid image name: %w", err)
	}
	// Validate container name
	if err := ValidateDockerName(c.ContainerName); err != nil {
		return fmt.Errorf("invalid container name: %w", err)
	}
	// Validate volume mount point
	if err := validatePath(c.VolumeMountPoint, "volume mount point"); err != nil {
		return err
	}
	// Validate workspace path
	if err := validatePath(c.WorkspacePath, "workspace path"); err != nil {
		return err
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
