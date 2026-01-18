package state

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeanhaley32/claude-capsule/internal/constants"
)

// Timeout for state detection commands
const stateCheckTimeout = 10 * time.Second

// EnvironmentState represents the current state of the capsule environment.
type EnvironmentState struct {
	VolumeExists     bool
	VolumePath       string
	VolumeMounted    bool
	MountPoint       string
	ContainerExists  bool
	ContainerRunning bool
	ContainerName    string
	SymlinkExists    bool
	SymlinkBroken    bool
	SymlinkPath      string
	WorkspacePath    string
}

// Detector checks the state of the environment.
type Detector struct {
	volumePath    string
	containerName string
	workspacePath string
}

// NewDetector creates a new state detector.
func NewDetector(volumePath, containerName, workspacePath string) *Detector {
	return &Detector{
		volumePath:    volumePath,
		containerName: containerName,
		workspacePath: workspacePath,
	}
}

// Detect checks all aspects of the environment state.
func (d *Detector) Detect() *EnvironmentState {
	state := &EnvironmentState{
		VolumePath:    d.volumePath,
		ContainerName: d.containerName,
		WorkspacePath: d.workspacePath,
	}

	// Check volume file exists
	if _, err := os.Stat(d.volumePath); err == nil {
		state.VolumeExists = true
	}

	// Check if volume is mounted
	state.MountPoint, state.VolumeMounted = d.checkVolumeMounted()

	// Check container status
	state.ContainerExists, state.ContainerRunning = d.checkContainer()

	// Check symlink status
	state.SymlinkPath = filepath.Join(d.workspacePath, constants.DocsSymlinkName)
	state.SymlinkExists, state.SymlinkBroken = d.checkSymlink()

	return state
}

// checkVolumeMounted checks if the ClaudeEnv volume is mounted.
// This mirrors the logic in volume/macos.go findMountPoint().
func (d *Detector) checkVolumeMounted() (string, bool) {
	// Check for unique mount points in /tmp (current approach)
	entries, err := os.ReadDir("/tmp")
	if err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "capsule-") && entry.IsDir() {
				mountPoint := filepath.Join("/tmp", entry.Name())
				// Verify it's actually mounted by checking for content
				contents, err := os.ReadDir(mountPoint)
				if err == nil && len(contents) > 0 {
					return mountPoint, true
				}
			}
		}
	}

	// Fall back to legacy /Volumes mount point
	if _, err := os.Stat(constants.MacOSMountPoint); err == nil {
		return constants.MacOSMountPoint, true
	}

	return "", false
}

// checkContainer checks if the container exists and is running.
func (d *Detector) checkContainer() (exists bool, running bool) {
	ctx, cancel := context.WithTimeout(context.Background(), stateCheckTimeout)
	defer cancel()

	// Check if container exists
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a", "-q", "-f", "name=^"+d.containerName+"$")
	output, err := cmd.Output()
	if err != nil || len(strings.TrimSpace(string(output))) == 0 {
		return false, false
	}

	exists = true

	// Check if container is running
	cmd = exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", d.containerName)
	output, err = cmd.Output()
	if err != nil {
		return exists, false
	}

	running = strings.TrimSpace(string(output)) == "true"
	return exists, running
}

// checkSymlink checks if the _docs symlink exists and if it's broken.
func (d *Detector) checkSymlink() (exists bool, broken bool) {
	symlinkPath := filepath.Join(d.workspacePath, constants.DocsSymlinkName)

	// Check if symlink exists
	info, err := os.Lstat(symlinkPath)
	if err != nil {
		return false, false
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return false, false // Not a symlink
	}

	exists = true

	// Check if target exists (broken symlink if not)
	if _, err := os.Stat(symlinkPath); os.IsNotExist(err) {
		broken = true
	}

	return exists, broken
}

// CheckDockerRunning verifies Docker daemon is running.
func CheckDockerRunning() error {
	ctx, cancel := context.WithTimeout(context.Background(), stateCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// CheckImageExists verifies a Docker image exists locally.
func CheckImageExists(imageName string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), stateCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", imageName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}
