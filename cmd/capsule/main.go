package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/jeanhaley32/claude-capsule/internal/constants"
	"github.com/jeanhaley32/claude-capsule/internal/docker"
	"github.com/jeanhaley32/claude-capsule/internal/embedded"
	"github.com/jeanhaley32/claude-capsule/internal/platform"
	"github.com/jeanhaley32/claude-capsule/internal/repo"
	"github.com/jeanhaley32/claude-capsule/internal/state"
	"github.com/jeanhaley32/claude-capsule/internal/terminal"
	"github.com/jeanhaley32/claude-capsule/internal/volume"
)

var version = "0.3.0"

// setupShutdownHandler registers signal handlers for graceful shutdown.
// Returns a cancel function that should be deferred to cleanup the handler.
// The cleanup function is ONLY called when a signal is received, not on normal exit.
func setupShutdownHandler(cleanup func()) func() {
	sigChan := make(chan os.Signal, 1)
	done := make(chan struct{})

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		select {
		case sig := <-sigChan:
			fmt.Fprintf(os.Stderr, "\nReceived signal: %v\n", sig)
			fmt.Fprintf(os.Stderr, "Cleaning up and locking volume...\n")
			cleanup()
			os.Exit(1)
		case <-done:
			// Normal exit - handler cancelled, do nothing
			return
		}
	}()

	return func() {
		signal.Stop(sigChan)
		close(done)
	}
}

// createShutdownCleanup creates a cleanup function that locks the specified volume.
func createShutdownCleanup(volumePath, containerName string) func() {
	return func() {
		volumeManager, err := volume.New()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create volume manager: %v\n", err)
			return
		}

		// Stop container if running
		dockerManager := docker.NewManager()
		if dockerManager.IsRunning(containerName) {
			fmt.Fprintf(os.Stderr, "Stopping container %s...\n", containerName)
			if err := dockerManager.Stop(containerName); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to stop container: %v\n", err)
			}
		}

		// Get the mount point for this specific volume (not any volume)
		mountPoint := volumeManager.GetMountPoint(volumePath)
		if mountPoint != "" {
			fmt.Fprintf(os.Stderr, "Locking volume at %s...\n", mountPoint)
			if err := volumeManager.Unmount(mountPoint); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to unmount volume: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "Volume locked successfully.\n")
			}
		}
	}
}

// getContainerNameForCwd returns the container name and current working directory.
// Returns (containerName, cwd, error).
func getContainerNameForCwd() (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to get current directory: %w", err)
	}

	repoIdentifier := repo.NewIdentifier()
	workspacePath, err := repoIdentifier.GetWorkspaceRoot(cwd)
	if err != nil {
		workspacePath = cwd
	}

	containerName, err := repoIdentifier.GetContainerName(workspacePath)
	if err != nil {
		return docker.DefaultContainerName, cwd, nil
	}

	return containerName, cwd, nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "capsule",
		Short: "Claude Capsule workspace environment",
		Long:  "A containerized, security-focused workspace for Claude Code with encrypted credential storage.",
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

	cmd.Flags().Int("size", 0, "Volume size in GB (prompts if not specified)")
	cmd.Flags().String("api-key", "", "Claude API key (optional, can be added later)")
	cmd.Flags().String("volume", "", "Explicit path for encrypted volume")
	cmd.Flags().Bool("local", false, "Create volume in current directory")
	cmd.Flags().Bool("global", false, "Create volume in ~/.capsule/volumes/ (default)")
	cmd.Flags().StringSlice("context", []string{}, "Markdown files to extend Claude context (can be specified multiple times)")
	cmd.Flags().Bool("with-memory", false, "Install doc-sync skill with SQLite memory system")

	return cmd
}

func runBootstrap(cmd *cobra.Command, args []string) error {
	size, err := cmd.Flags().GetInt("size")
	if err != nil {
		return fmt.Errorf("invalid size flag: %w", err)
	}
	apiKey, err := cmd.Flags().GetString("api-key")
	if err != nil {
		return fmt.Errorf("invalid api-key flag: %w", err)
	}
	volumePathFlag, err := cmd.Flags().GetString("volume")
	if err != nil {
		return fmt.Errorf("invalid volume flag: %w", err)
	}
	localFlag, err := cmd.Flags().GetBool("local")
	if err != nil {
		return fmt.Errorf("invalid local flag: %w", err)
	}
	globalFlag, err := cmd.Flags().GetBool("global")
	if err != nil {
		return fmt.Errorf("invalid global flag: %w", err)
	}
	contextFiles, err := cmd.Flags().GetStringSlice("context")
	if err != nil {
		return fmt.Errorf("invalid context flag: %w", err)
	}
	withMemory, err := cmd.Flags().GetBool("with-memory")
	if err != nil {
		return fmt.Errorf("invalid with-memory flag: %w", err)
	}

	// Convert context files to absolute paths
	for i, ctxFile := range contextFiles {
		if !filepath.IsAbs(ctxFile) {
			absCtxPath, err := filepath.Abs(ctxFile)
			if err != nil {
				return fmt.Errorf("invalid context file path %s: %w", ctxFile, err)
			}
			contextFiles[i] = absCtxPath
		}
	}

	// Create path resolver
	pathResolver, err := volume.NewPathResolver()
	if err != nil {
		return fmt.Errorf("failed to create path resolver: %w", err)
	}

	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Determine volume path based on flags or interactive prompt
	var volumePath string
	locationSpecified := volumePathFlag != "" || localFlag || globalFlag

	if volumePathFlag != "" {
		// Explicit path provided
		volumePath = volumePathFlag
		if !filepath.IsAbs(volumePath) {
			volumePath, err = filepath.Abs(volumePath)
			if err != nil {
				return fmt.Errorf("invalid volume path: %w", err)
			}
		}
	} else if localFlag {
		// Local flag
		volumePath = pathResolver.GetLocalVolumePath(cwd)
	} else if globalFlag {
		// Global flag
		volumePath = pathResolver.GetDefaultVolumePath()
	} else {
		// Interactive prompt for location
		options := []string{
			"Global (~/.capsule/volumes/) - accessible from any project (Recommended)",
			"Local (./capsule.sparseimage) - specific to this directory",
		}
		choice, err := terminal.PromptChoice("Where should the encrypted volume be stored?", options, 0)
		if err != nil {
			return fmt.Errorf("failed to get location choice: %w", err)
		}
		if choice == 0 {
			volumePath = pathResolver.GetDefaultVolumePath()
		} else {
			volumePath = pathResolver.GetLocalVolumePath(cwd)
		}
	}

	// Prompt for size if not specified
	if size == 0 {
		if locationSpecified {
			// If location was specified via flag, use default size
			size = 2
		} else {
			// Interactive prompt for size
			size, err = terminal.PromptIntWithDefault("Volume size in GB", 2)
			if err != nil {
				return fmt.Errorf("failed to get size: %w", err)
			}
		}
	}

	// Validate size
	if size < constants.MinVolumeSizeGB || size > constants.MaxVolumeSizeGB {
		return fmt.Errorf("volume size must be between %d and %d GB", constants.MinVolumeSizeGB, constants.MaxVolumeSizeGB)
	}

	// Create volume manager
	volumeManager, err := volume.New()
	if err != nil {
		return fmt.Errorf("failed to create volume manager: %w", err)
	}

	// Check if volume already exists
	if volumeManager.Exists(volumePath) {
		return fmt.Errorf("already bootstrapped: volume exists at %s\nUse 'capsule start' to begin a session", volumePath)
	}

	// Prompt for password
	password, err := terminal.ReadPasswordConfirmSecure(
		"Enter encryption password: ",
		"Confirm password: ",
	)
	if err != nil {
		return fmt.Errorf("password error: %w", err)
	}
	defer password.Clear()

	fmt.Printf("Creating encrypted volume at %s...\n", volumePath)

	// Bootstrap the volume
	cfg := volume.BootstrapConfig{
		VolumePath:   volumePath,
		SizeGB:       size,
		Password:     password,
		ContextFiles: contextFiles,
		WithMemory:   withMemory,
		Version:      version,
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
	fmt.Println("Next step:")
	fmt.Println("  capsule start")

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
	volumePathFlag, err := cmd.Flags().GetString("volume")
	if err != nil {
		return fmt.Errorf("invalid volume flag: %w", err)
	}
	workspaceFlag, err := cmd.Flags().GetString("workspace")
	if err != nil {
		return fmt.Errorf("invalid workspace flag: %w", err)
	}

	// Get current directory once for reuse
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Create managers
	volumeManager, err := volume.New()
	if err != nil {
		return fmt.Errorf("failed to create volume manager: %w", err)
	}
	dockerManager := docker.NewManager()
	repoIdentifier := repo.NewIdentifier()

	// Create path resolver
	pathResolver, err := volume.NewPathResolver()
	if err != nil {
		return fmt.Errorf("failed to create path resolver: %w", err)
	}

	// Find volume path using priority rules
	volumePath, err := pathResolver.ResolveVolumePathStrict(volumePathFlag, cwd)
	if err != nil {
		return err
	}

	// Determine workspace
	workspacePath := workspaceFlag
	if workspacePath == "" {
		workspacePath, err = repoIdentifier.GetWorkspaceRoot(cwd)
		if err != nil {
			return fmt.Errorf("failed to determine workspace root: %w", err)
		}
	}
	workspacePath, err = filepath.Abs(workspacePath)
	if err != nil {
		return fmt.Errorf("failed to resolve workspace path: %w", err)
	}

	// Get repo ID for symlink and container name
	repoID, err := repoIdentifier.GetRepoID(workspacePath)
	if err != nil {
		return fmt.Errorf("failed to identify repository: %w", err)
	}

	// Get unique container name for this workspace
	containerName, err := repoIdentifier.GetContainerName(workspacePath)
	if err != nil {
		return fmt.Errorf("failed to generate container name: %w", err)
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
		return fmt.Errorf("Docker file sharing check failed: %w", err)
	}

	// Pre-start cleanup: remove any stale container from previous runs
	// This prevents Docker mount conflicts even with stopped containers
	fmt.Println("Checking for stale containers...")
	if err := dockerManager.RemoveContainer(containerName); err == nil {
		fmt.Println("Removed stale container.")
		time.Sleep(docker.MountReleaseDelay)
	}

	// Check if volume is already mounted (reuse existing mount for fast re-entry)
	var mountPoint string
	var password *terminal.SecurePassword
	if existingMount := volumeManager.GetMountPoint(volumePath); existingMount != "" {
		fmt.Printf("Volume already mounted at %s\n", existingMount)
		mountPoint = existingMount
	} else {
		// Prompt for password only when we need to mount
		password, err = terminal.ReadPasswordSecure("Enter volume password: ")
		if err != nil {
			return fmt.Errorf("password error: %w", err)
		}
		defer password.Clear()

		// Mount volume
		fmt.Println("Mounting encrypted volume...")
		mountPoint, err = volumeManager.Mount(volumePath, password)
		if err != nil {
			return fmt.Errorf("failed to mount volume: %w", err)
		}
		fmt.Printf("Volume mounted at %s\n", mountPoint)
	}

	// Setup shutdown handler to lock volume on crash/termination
	// This ensures the volume is secured if the process is killed unexpectedly
	cancelShutdown := setupShutdownHandler(createShutdownCleanup(volumePath, containerName))
	defer cancelShutdown()

	// Clear VM cache and refresh Docker's VirtioFS view of the mount point
	// This is necessary because Docker Desktop caches mount information,
	// and freshly mounted volumes may not be visible without cache clearing
	fmt.Println("Preparing Docker mount...")
	if err := dockerManager.ClearVMCache(); err != nil {
		// Non-fatal: log warning but continue
		fmt.Fprintf(os.Stderr, "Warning: failed to clear VM cache: %v\n", err)
	}
	if err := dockerManager.RefreshMountCache(mountPoint); err != nil {
		// Non-fatal: if refresh fails, the actual mount will report a clearer error
		fmt.Fprintf(os.Stderr, "Warning: cache refresh failed (will retry on mount): %v\n", err)
	}

	// Start container with retry on Docker mount cache errors
	fmt.Println("Starting container...")
	containerConfig := docker.ContainerConfig{
		ImageName:        docker.DefaultImageName,
		ContainerName:    containerName,
		VolumeMountPoint: mountPoint,
		WorkspacePath:    workspacePath,
	}

	startErr := dockerManager.Start(containerConfig)
	if startErr != nil && strings.Contains(startErr.Error(), "file exists") {
		// Docker Desktop has stale mount cache - clean up and retry
		fmt.Println("Docker mount cache conflict detected, cleaning up...")

		// Remove any partial container (errors ignored - container may not exist)
		if err := dockerManager.RemoveContainer(containerName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: container removal failed: %v\n", err)
		}

		// Unmount and remove mount directory (Unmount now handles directory cleanup)
		if err := volumeManager.Unmount(mountPoint); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: volume unmount failed: %v\n", err)
		}

		// Wait for Docker Desktop to clear its cache
		fmt.Println("Waiting for Docker to refresh...")
		time.Sleep(docker.CacheRefreshDelay)

		// If we didn't have a password (volume was pre-mounted), prompt now
		if password == nil {
			password, err = terminal.ReadPasswordSecure("Enter volume password to remount: ")
			if err != nil {
				return fmt.Errorf("password error: %w", err)
			}
			defer password.Clear()
		}

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
		fmt.Println("Cleaning up failed container...")
		if err := dockerManager.RemoveContainer(containerName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: container removal failed: %v\n", err)
		}

		if unmountErr := volumeManager.Unmount(mountPoint); unmountErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: volume unmount failed: %v\n", unmountErr)
		}
		return fmt.Errorf("failed to start container: %w", startErr)
	}
	fmt.Println("Container started!")

	// Setup symlink inside container
	fmt.Println("Setting up shadow documentation...")
	if err := dockerManager.SetupWorkspaceSymlink(containerName, repoID); err != nil {
		// Clean up on failure
		if stopErr := dockerManager.Stop(containerName); stopErr != nil {
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
	execErr := dockerManager.Exec(containerName)

	// Clean up after user exits the shell
	fmt.Println("")
	fmt.Println("Cleaning up...")

	// Stop container (keep volume mounted for fast re-entry)
	if err := dockerManager.Stop(containerName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to stop container: %v\n", err)
	} else {
		fmt.Println("Container stopped.")
	}

	fmt.Println("Volume remains unlocked for quick re-entry.")
	fmt.Println("Run 'capsule lock' when done to secure your credentials.")

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
  MOUNT_POINT=/tmp/capsule-abc123
  STATUS=mounted

Password can be provided via:
  - Interactive prompt (default)
  - --password-stdin flag: echo $PASS | capsule unlock --password-stdin
  - CAPSULE_PASSWORD environment variable`,
		RunE: runUnlock,
	}

	cmd.Flags().String("volume", "", "Path to encrypted volume (auto-detected if not specified)")
	cmd.Flags().Bool("password-stdin", false, "Read password from stdin instead of terminal prompt")

	return cmd
}

func runUnlock(cmd *cobra.Command, args []string) error {
	volumePathFlag, err := cmd.Flags().GetString("volume")
	if err != nil {
		return fmt.Errorf("invalid volume flag: %w", err)
	}
	passwordStdin, err := cmd.Flags().GetBool("password-stdin")
	if err != nil {
		return fmt.Errorf("invalid password-stdin flag: %w", err)
	}

	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Create volume manager
	volumeManager, err := volume.New()
	if err != nil {
		return fmt.Errorf("failed to create volume manager: %w", err)
	}

	// Create path resolver
	pathResolver, err := volume.NewPathResolver()
	if err != nil {
		return fmt.Errorf("failed to create path resolver: %w", err)
	}

	// Find volume path using priority rules
	volumePath, err := pathResolver.ResolveVolumePathStrict(volumePathFlag, cwd)
	if err != nil {
		return err
	}

	// Check if already mounted
	if existingMount := volumeManager.GetMountPoint(volumePath); existingMount != "" {
		// Output parsable values
		fmt.Printf("MOUNT_POINT=%s\n", existingMount)
		fmt.Printf("STATUS=already_mounted\n")
		fmt.Printf("VOLUME_PATH=%s\n", volumePath)
		return nil
	}

	// Get password from multiple sources
	password, err := terminal.ReadPasswordMultiSourceSecure(passwordStdin, "Enter volume password: ")
	if err != nil {
		return fmt.Errorf("password error: %w", err)
	}
	defer password.Clear()

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

	fmt.Fprintf(os.Stderr, "Volume unlocked. Run 'capsule lock' to secure.\n")
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

	cmd.Flags().String("volume", "", "Path to encrypted volume (auto-detected if not specified)")

	return cmd
}

func runLock(cmd *cobra.Command, args []string) error {
	volumePathFlag, err := cmd.Flags().GetString("volume")
	if err != nil {
		return fmt.Errorf("invalid volume flag: %w", err)
	}

	// Get container name and cwd for current directory
	containerName, cwd, err := getContainerNameForCwd()
	if err != nil {
		return err
	}

	// Create volume manager
	volumeManager, err := volume.New()
	if err != nil {
		return fmt.Errorf("failed to create volume manager: %w", err)
	}

	// Create path resolver
	pathResolver, err := volume.NewPathResolver()
	if err != nil {
		return fmt.Errorf("failed to create path resolver: %w", err)
	}

	// Find volume path using priority rules (allow non-existent for status reporting)
	volumePath, _ := pathResolver.ResolveVolumePath(volumePathFlag, cwd)

	// Get the mount point for this specific volume (not any volume)
	mountPoint := volumeManager.GetMountPoint(volumePath)
	if mountPoint == "" {
		fmt.Printf("STATUS=not_mounted\n")
		fmt.Printf("VOLUME_PATH=%s\n", volumePath)
		fmt.Fprintf(os.Stderr, "Volume is not mounted. Nothing to lock.\n")
		return nil
	}

	// Stop any running container first
	dockerManager := docker.NewManager()
	if dockerManager.IsRunning(containerName) {
		fmt.Fprintf(os.Stderr, "Stopping running container %s...\n", containerName)
		if err := dockerManager.Stop(containerName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop container: %v\n", err)
		}
	}

	// Unmount the specific volume
	fmt.Fprintf(os.Stderr, "Unmounting encrypted volume at %s...\n", mountPoint)
	if err := volumeManager.Unmount(mountPoint); err != nil {
		return fmt.Errorf("failed to unmount volume: %w", err)
	}

	// Output parsable values to stdout
	fmt.Printf("STATUS=locked\n")
	fmt.Printf("VOLUME_PATH=%s\n", volumePath)

	fmt.Fprintf(os.Stderr, "Volume locked. Your credentials are now secured.\n")
	return nil
}

func runStop(cmd *cobra.Command, args []string) error {
	// Get container name for current directory
	containerName, _, err := getContainerNameForCwd()
	if err != nil {
		return err
	}

	dockerManager := docker.NewManager()

	// Stop container (symlink inside container is destroyed with it)
	fmt.Printf("Stopping container %s...\n", containerName)
	if err := dockerManager.Stop(containerName); err != nil {
		fmt.Printf("Warning: Failed to stop container: %v\n", err)
	} else {
		fmt.Println("Container stopped.")
	}

	// Keep volume mounted for quick re-entry
	fmt.Println("Volume remains mounted. Run 'capsule lock' to unmount and secure.")
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

	// Get container name and cwd for current directory
	containerName, cwd, err := getContainerNameForCwd()
	if err != nil {
		return err
	}

	// Create path resolver
	pathResolver, err := volume.NewPathResolver()
	if err != nil {
		return fmt.Errorf("failed to create path resolver: %w", err)
	}

	// Find volume path using priority rules (allow non-existent for status display)
	volumePath, _ := pathResolver.ResolveVolumePath(volumePathFlag, cwd)

	// Create detector
	detector := state.NewDetector(volumePath, containerName, cwd)
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
		fmt.Println("Build it with: capsule build-image")
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
			fmt.Printf("capsule version %s\n", version)
			fmt.Printf("Platform: %s\n", platform.Detect())
		},
	}
}
