package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jeanhaley32/portable-claude-env/internal/constants"
	"github.com/jeanhaley32/portable-claude-env/internal/docker"
	"github.com/jeanhaley32/portable-claude-env/internal/embedded"
	"github.com/jeanhaley32/portable-claude-env/internal/platform"
	"github.com/jeanhaley32/portable-claude-env/internal/repo"
	"github.com/jeanhaley32/portable-claude-env/internal/state"
	"github.com/jeanhaley32/portable-claude-env/internal/terminal"
	"github.com/jeanhaley32/portable-claude-env/internal/volume"
)

var version = "0.2.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "claude-env",
		Short: "Portable Claude Code development environment",
		Long:  "A containerized, security-focused development environment for Claude Code with encrypted credential storage.",
	}

	rootCmd.AddCommand(
		newBootstrapCmd(),
		newStartCmd(),
		newStopCmd(),
		newUnlockCmd(),
		newLockCmd(),
		newStatusCmd(),
		newBuildImageCmd(),
		newVersionCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newBootstrapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Create new encrypted volume and initialize environment",
		RunE:  runBootstrap,
	}

	cmd.Flags().Int("size", 2, "Volume size in GB")
	cmd.Flags().String("api-key", "", "Claude API key (optional, can be added later)")
	cmd.Flags().String("path", ".", "Path for encrypted volume")

	return cmd
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	// Check platform
	if !platform.IsMacOS() {
		return fmt.Errorf("bootstrap currently only supports macOS")
	}

	size, err := cmd.Flags().GetInt("size")
	if err != nil {
		return fmt.Errorf("invalid size flag: %w", err)
	}
	apiKey, err := cmd.Flags().GetString("api-key")
	if err != nil {
		return fmt.Errorf("invalid api-key flag: %w", err)
	}
	basePath, err := cmd.Flags().GetString("path")
	if err != nil {
		return fmt.Errorf("invalid path flag: %w", err)
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Create volume manager
	volumeManager, err := volume.New()
	if err != nil {
		return err
	}

	volumePath := volumeManager.GetVolumePath(absPath)

	// Check if volume already exists
	if volumeManager.Exists(volumePath) {
		return fmt.Errorf("volume already exists at %s", volumePath)
	}

	// Prompt for password
	password, err := terminal.ReadPasswordConfirm(
		"Enter encryption password: ",
		"Confirm password: ",
	)
	if err != nil {
		return fmt.Errorf("password error: %w", err)
	}

	fmt.Printf("Creating encrypted volume at %s...\n", volumePath)

	// Bootstrap the volume
	cfg := volume.BootstrapConfig{
		Path:     absPath,
		SizeGB:   size,
		Password: password,
	}

	if err := volumeManager.Bootstrap(cfg); err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}

	// If API key provided, write it to the volume
	if apiKey != "" {
		// Mount volume to write API key
		mountPoint, err := volumeManager.Mount(volumePath, password)
		if err != nil {
			fmt.Printf("Warning: Could not mount volume to save API key: %v\n", err)
		} else {
			apiKeyPath := filepath.Join(mountPoint, "auth", "api-key")
			if err := os.WriteFile(apiKeyPath, []byte(apiKey), constants.FilePermissions); err != nil {
				fmt.Printf("Warning: Could not save API key: %v\n", err)
			}
			if err := volumeManager.Unmount(mountPoint); err != nil {
				fmt.Printf("Warning: Could not unmount volume after saving API key: %v\n", err)
			}
		}
	}

	fmt.Println("Volume created successfully!")
	fmt.Println("")
	fmt.Println("Next steps:")
	fmt.Println("  1. Build the Docker image: docker build -t portable-claude:latest .")
	fmt.Println("  2. Start the environment: claude-env start")

	return nil
}

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Mount volume and start container",
		RunE:  runStart,
	}

	cmd.Flags().String("volume", "", "Path to encrypted volume (auto-detected if not specified)")
	cmd.Flags().String("workspace", "", "Workspace path (defaults to current directory or git root)")

	return cmd
}

func runStart(cmd *cobra.Command, args []string) error {
	// Check platform
	if !platform.IsMacOS() {
		return fmt.Errorf("start currently only supports macOS")
	}

	volumePathFlag, err := cmd.Flags().GetString("volume")
	if err != nil {
		return fmt.Errorf("invalid volume flag: %w", err)
	}
	workspaceFlag, err := cmd.Flags().GetString("workspace")
	if err != nil {
		return fmt.Errorf("invalid workspace flag: %w", err)
	}

	// Create managers
	volumeManager, err := volume.New()
	if err != nil {
		return err
	}
	dockerManager := docker.NewManager()
	repoIdentifier := repo.NewIdentifier()

	// Find volume path
	volumePath := volumePathFlag
	if volumePath == "" {
		// Look in current directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		volumePath = volumeManager.GetVolumePath(cwd)
		if !volumeManager.Exists(volumePath) {
			return fmt.Errorf("volume not found at %s. Run 'claude-env bootstrap' first or specify --volume", volumePath)
		}
	}

	// Determine workspace
	workspacePath := workspaceFlag
	if workspacePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		workspacePath, err = repoIdentifier.GetWorkspaceRoot(cwd)
		if err != nil {
			return fmt.Errorf("failed to determine workspace root: %w", err)
		}
	}
	workspacePath, err = filepath.Abs(workspacePath)
	if err != nil {
		return fmt.Errorf("failed to resolve workspace path: %w", err)
	}

	// Get repo ID for symlink
	repoID, err := repoIdentifier.GetRepoID(workspacePath)
	if err != nil {
		return fmt.Errorf("failed to identify repository: %w", err)
	}

	// Prompt for password
	password, err := terminal.ReadPassword("Enter volume password: ")
	if err != nil {
		return fmt.Errorf("password error: %w", err)
	}

	// Check if Docker image exists, build if needed
	if !embedded.ImageExists(docker.DefaultImageName) {
		fmt.Printf("Docker image '%s' not found. Building...\n", docker.DefaultImageName)
		if err := embedded.BuildImage(docker.DefaultImageName); err != nil {
			return fmt.Errorf("failed to build Docker image: %w", err)
		}
		fmt.Println("Docker image built successfully!")
	}

	// Verify Docker Desktop can access /tmp for encrypted volume mounts
	fmt.Println("Checking Docker file sharing configuration...")
	if err := dockerManager.CheckTmpFileSharing(); err != nil {
		return err
	}

	// Pre-start cleanup: remove any stale container from previous runs
	// This prevents Docker mount conflicts even with stopped containers
	// Use docker rm -f directly for reliable cleanup regardless of container state
	fmt.Println("Checking for stale containers...")
	cleanupCmd := exec.Command("docker", "rm", "-f", docker.DefaultContainerName)
	cleanupOutput, cleanupErr := cleanupCmd.CombinedOutput()
	if cleanupErr != nil {
		fmt.Fprintf(os.Stderr, "[pre-start] No container to remove (or error): %s\n", strings.TrimSpace(string(cleanupOutput)))
	} else {
		fmt.Fprintf(os.Stderr, "[pre-start] Removed stale container: %s\n", strings.TrimSpace(string(cleanupOutput)))
		// Give Docker time to release mount references
		fmt.Fprintf(os.Stderr, "[pre-start] Waiting for Docker to release mount references...\n")
		time.Sleep(docker.MountReleaseDelay)
	}

	// Check if volume is already mounted (reuse existing mount for fast re-entry)
	var mountPoint string
	if existingMount := volumeManager.GetMountPoint(); existingMount != "" {
		fmt.Printf("Volume already mounted at %s\n", existingMount)
		mountPoint = existingMount
	} else {
		// Mount volume
		fmt.Println("Mounting encrypted volume...")
		mountPoint, err = volumeManager.Mount(volumePath, password)
		if err != nil {
			return fmt.Errorf("failed to mount volume: %w", err)
		}
		fmt.Printf("Volume mounted at %s\n", mountPoint)
	}

	// Refresh Docker's VirtioFS cache for the mount point
	// This is necessary because Docker Desktop caches mount information
	fmt.Println("Preparing Docker mount...")
	if err := dockerManager.RefreshMountCache(mountPoint); err != nil {
		// Non-fatal: if refresh fails, the actual mount will report a clearer error
		fmt.Fprintf(os.Stderr, "[warning] Cache refresh failed (will retry on mount): %v\n", err)
	}

	// Start container with retry on Docker mount cache errors
	fmt.Println("Starting container...")
	containerConfig := docker.ContainerConfig{
		ImageName:        docker.DefaultImageName,
		ContainerName:    docker.DefaultContainerName,
		VolumeMountPoint: mountPoint,
		WorkspacePath:    workspacePath,
	}

	startErr := dockerManager.Start(containerConfig)
	if startErr != nil && strings.Contains(startErr.Error(), "file exists") {
		// Docker Desktop has stale mount cache - clean up and retry
		fmt.Println("Docker mount cache conflict detected, cleaning up...")

		// Remove any partial container (errors ignored - container may not exist)
		retryCleanupCmd := exec.Command("docker", "rm", "-f", docker.DefaultContainerName)
		if err := retryCleanupCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "[cleanup] Container removal: %v\n", err)
		}

		// Unmount and remove mount directory (Unmount now handles directory cleanup)
		if err := volumeManager.Unmount(mountPoint); err != nil {
			fmt.Fprintf(os.Stderr, "[cleanup] Volume unmount: %v\n", err)
		}

		// Wait for Docker Desktop to clear its cache
		fmt.Println("Waiting for Docker to refresh...")
		time.Sleep(docker.CacheRefreshDelay)

		// Remount
		fmt.Println("Remounting volume...")
		mountPoint, err = volumeManager.Mount(volumePath, password)
		if err != nil {
			return fmt.Errorf("failed to remount volume after cleanup: %w", err)
		}

		// Update config with new mount point
		containerConfig.VolumeMountPoint = mountPoint

		// Retry start
		fmt.Println("Retrying container start...")
		startErr = dockerManager.Start(containerConfig)
	}

	if startErr != nil {
		// Clean up any partially created container before returning error
		fmt.Fprintf(os.Stderr, "[error] Cleaning up failed container...\n")
		failCleanupCmd := exec.Command("docker", "rm", "-f", docker.DefaultContainerName)
		if err := failCleanupCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "[cleanup] Container removal: %v\n", err)
		}

		if unmountErr := volumeManager.Unmount(mountPoint); unmountErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: cleanup failed to unmount volume: %v\n", unmountErr)
		}
		return fmt.Errorf("failed to start container: %w", startErr)
	}
	fmt.Println("Container started!")

	// Setup symlink inside container
	fmt.Println("Setting up shadow documentation...")
	if err := dockerManager.SetupWorkspaceSymlink(docker.DefaultContainerName, repoID); err != nil {
		// Clean up on failure
		if stopErr := dockerManager.Stop(docker.DefaultContainerName); stopErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: cleanup failed to stop container: %v\n", stopErr)
		}
		if unmountErr := volumeManager.Unmount(mountPoint); unmountErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: cleanup failed to unmount volume: %v\n", unmountErr)
		}
		return fmt.Errorf("failed to setup workspace symlink: %w", err)
	}
	fmt.Println("")
	fmt.Println("Entering container... (type 'exit' to leave)")
	fmt.Println("")

	// Exec into container and wait for user to exit
	execErr := dockerManager.Exec(docker.DefaultContainerName)

	// Clean up after user exits the shell
	fmt.Println("")
	fmt.Println("Cleaning up...")

	// Stop container (keep volume mounted for fast re-entry)
	if err := dockerManager.Stop(docker.DefaultContainerName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to stop container: %v\n", err)
	} else {
		fmt.Println("Container stopped.")
	}

	fmt.Println("Volume remains unlocked for quick re-entry.")
	fmt.Println("Run 'claude-env lock' when done to secure your credentials.")

	// Ignore common exit codes (0 = normal, 130 = Ctrl+C)
	if execErr != nil {
		if exitErr, ok := execErr.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			if code == 0 || code == 130 {
				return nil
			}
		}
		return fmt.Errorf("shell exited with error: %w", execErr)
	}

	return nil
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop container (keeps volume mounted)",
		RunE:  runStop,
	}
}

func newUnlockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "Mount encrypted volume without starting container",
		Long: `Mounts the encrypted volume to the host filesystem without starting Docker.
This allows injecting files or accessing the volume from external processes.

Output is in KEY=VALUE format for easy parsing:
  MOUNT_POINT=/tmp/claude-env-abc123
  STATUS=mounted

Password can be provided via:
  - Interactive prompt (default)
  - --password-stdin flag: echo $PASS | claude-env unlock --password-stdin
  - CLAUDE_ENV_PASSWORD environment variable`,
		RunE: runUnlock,
	}

	cmd.Flags().String("volume", "", "Path to encrypted volume (auto-detected if not specified)")
	cmd.Flags().Bool("password-stdin", false, "Read password from stdin instead of terminal prompt")

	return cmd
}

func runUnlock(cmd *cobra.Command, args []string) error {
	// Check platform
	if !platform.IsMacOS() {
		return fmt.Errorf("unlock currently only supports macOS")
	}

	volumePathFlag, err := cmd.Flags().GetString("volume")
	if err != nil {
		return fmt.Errorf("invalid volume flag: %w", err)
	}
	passwordStdin, err := cmd.Flags().GetBool("password-stdin")
	if err != nil {
		return fmt.Errorf("invalid password-stdin flag: %w", err)
	}

	// Create volume manager
	volumeManager, err := volume.New()
	if err != nil {
		return err
	}

	// Find volume path
	volumePath := volumePathFlag
	if volumePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		volumePath = volumeManager.GetVolumePath(cwd)
		if !volumeManager.Exists(volumePath) {
			return fmt.Errorf("volume not found at %s. Run 'claude-env bootstrap' first or specify --volume", volumePath)
		}
	}

	// Check if already mounted
	if existingMount := volumeManager.GetMountPoint(); existingMount != "" {
		// Output parsable values
		fmt.Printf("MOUNT_POINT=%s\n", existingMount)
		fmt.Printf("STATUS=already_mounted\n")
		fmt.Printf("VOLUME_PATH=%s\n", volumePath)
		return nil
	}

	// Get password from multiple sources
	password, err := terminal.ReadPasswordMultiSource(passwordStdin, "Enter volume password: ")
	if err != nil {
		return fmt.Errorf("password error: %w", err)
	}

	// Mount volume
	fmt.Fprintf(os.Stderr, "Mounting encrypted volume...\n")
	mountPoint, err := volumeManager.Mount(volumePath, password)
	if err != nil {
		return fmt.Errorf("failed to mount volume: %w", err)
	}

	// Output parsable values to stdout
	fmt.Printf("MOUNT_POINT=%s\n", mountPoint)
	fmt.Printf("STATUS=mounted\n")
	fmt.Printf("VOLUME_PATH=%s\n", volumePath)

	fmt.Fprintf(os.Stderr, "Volume unlocked. Run 'claude-env lock' to secure.\n")
	return nil
}

func newLockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lock",
		Short: "Unmount encrypted volume to secure credentials",
		Long: `Unmounts the encrypted volume, securing all credentials and data.
Use this when you're done working for the day.

Output is in KEY=VALUE format for easy parsing:
  STATUS=locked`,
		RunE: runLock,
	}

	return cmd
}

func runLock(cmd *cobra.Command, args []string) error {
	// Create volume manager
	volumeManager, err := volume.New()
	if err != nil {
		return fmt.Errorf("failed to create volume manager: %w", err)
	}

	// Check if volume is mounted
	if !volumeManager.IsMounted() {
		fmt.Printf("STATUS=not_mounted\n")
		fmt.Fprintf(os.Stderr, "Volume is not mounted. Nothing to lock.\n")
		return nil
	}

	// Stop any running container first
	dockerManager := docker.NewManager()
	if dockerManager.IsRunning(docker.DefaultContainerName) {
		fmt.Fprintf(os.Stderr, "Stopping running container...\n")
		if err := dockerManager.Stop(docker.DefaultContainerName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop container: %v\n", err)
		}
	}

	// Unmount the volume (this also clears VM cache via drop_caches)
	fmt.Fprintf(os.Stderr, "Unmounting encrypted volume...\n")
	if err := volumeManager.Unmount(""); err != nil {
		return fmt.Errorf("failed to unmount volume: %w", err)
	}

	// Output parsable values to stdout
	fmt.Printf("STATUS=locked\n")

	fmt.Fprintf(os.Stderr, "Volume locked. Your credentials are now secured.\n")
	return nil
}

func runStop(cmd *cobra.Command, args []string) error {
	dockerManager := docker.NewManager()

	// Stop container (symlink inside container is destroyed with it)
	fmt.Println("Stopping container...")
	if err := dockerManager.Stop(docker.DefaultContainerName); err != nil {
		fmt.Printf("Warning: Failed to stop container: %v\n", err)
	} else {
		fmt.Println("Container stopped.")
	}

	// Keep volume mounted for quick re-entry
	fmt.Println("Volume remains mounted. Run 'claude-env lock' to unmount and secure.")
	return nil
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show environment status",
		RunE:  runStatus,
	}

	cmd.Flags().String("volume", "", "Path to encrypted volume")

	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	volumePathFlag, err := cmd.Flags().GetString("volume")
	if err != nil {
		return fmt.Errorf("invalid volume flag: %w", err)
	}

	// Determine paths
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	volumeManager, err := volume.New()
	if err != nil {
		return fmt.Errorf("failed to create volume manager: %w", err)
	}

	volumePath := volumePathFlag
	if volumePath == "" {
		volumePath = volumeManager.GetVolumePath(cwd)
	}

	// Create detector
	detector := state.NewDetector(volumePath, docker.DefaultContainerName, cwd)
	envState := detector.Detect()

	// Display status
	fmt.Println("Claude Environment Status")
	fmt.Println("=========================")
	fmt.Println()

	// Volume status
	if envState.VolumeExists {
		fmt.Printf("Volume:     %s (exists)\n", envState.VolumePath)
	} else {
		fmt.Printf("Volume:     %s (not found)\n", volumePath)
	}

	// Mount status
	if envState.VolumeMounted {
		fmt.Printf("Mounted:    Yes (%s)\n", envState.MountPoint)
	} else {
		fmt.Println("Mounted:    No")
	}

	// Container status
	if envState.ContainerRunning {
		fmt.Printf("Container:  Running (%s)\n", envState.ContainerName)
	} else if envState.ContainerExists {
		fmt.Printf("Container:  Stopped (%s)\n", envState.ContainerName)
	} else {
		fmt.Println("Container:  Not created")
	}

	// Symlink status
	if envState.SymlinkExists {
		if envState.SymlinkBroken {
			fmt.Printf("Symlink:    Broken (%s)\n", envState.SymlinkPath)
		} else {
			fmt.Printf("Symlink:    Active (%s)\n", envState.SymlinkPath)
		}
	} else {
		fmt.Println("Symlink:    Not created")
	}

	// Docker status
	if err := state.CheckDockerRunning(); err != nil {
		fmt.Println("\nWarning: Docker is not running!")
	}

	// Image status
	if !state.CheckImageExists(docker.DefaultImageName) {
		fmt.Printf("\nWarning: Docker image '%s' not found.\n", docker.DefaultImageName)
		fmt.Println("Build it with: claude-env build-image")
	}

	return nil
}

func newBuildImageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build-image",
		Short: "Build the Docker image",
		Long:  "Build the Docker image from the embedded Dockerfile. This is done automatically on first start.",
		RunE:  runBuildImage,
	}

	cmd.Flags().Bool("force", false, "Rebuild even if image already exists")

	return cmd
}

func runBuildImage(cmd *cobra.Command, args []string) error {
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return fmt.Errorf("invalid force flag: %w", err)
	}

	if !force && embedded.ImageExists(docker.DefaultImageName) {
		fmt.Printf("Docker image '%s' already exists. Use --force to rebuild.\n", docker.DefaultImageName)
		return nil
	}

	fmt.Printf("Building Docker image '%s'...\n", docker.DefaultImageName)
	if err := embedded.BuildImage(docker.DefaultImageName); err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	fmt.Println("Docker image built successfully!")
	return nil
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("claude-env version %s\n", version)
			fmt.Printf("Platform: %s\n", platform.Detect())
		},
	}
}
