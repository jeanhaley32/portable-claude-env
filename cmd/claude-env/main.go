package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jeanhaley32/portable-claude-env/internal/constants"
	"github.com/jeanhaley32/portable-claude-env/internal/docker"
	"github.com/jeanhaley32/portable-claude-env/internal/platform"
	"github.com/jeanhaley32/portable-claude-env/internal/repo"
	"github.com/jeanhaley32/portable-claude-env/internal/state"
	"github.com/jeanhaley32/portable-claude-env/internal/terminal"
	"github.com/jeanhaley32/portable-claude-env/internal/volume"
)

var version = "0.1.0"

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
		newStatusCmd(),
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

	if len(password) < constants.MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", constants.MinPasswordLength)
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

	// Mount volume
	fmt.Println("Mounting encrypted volume...")
	mountPoint, err := volumeManager.Mount(volumePath, password)
	if err != nil {
		return fmt.Errorf("failed to mount volume: %w", err)
	}
	fmt.Printf("Volume mounted at %s\n", mountPoint)

	// Start container
	fmt.Println("Starting container...")
	containerConfig := docker.ContainerConfig{
		ImageName:        docker.DefaultImageName,
		ContainerName:    docker.DefaultContainerName,
		VolumeMountPoint: mountPoint,
		WorkspacePath:    workspacePath,
	}

	if err := dockerManager.Start(containerConfig); err != nil {
		if unmountErr := volumeManager.Unmount(mountPoint); unmountErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: cleanup failed to unmount volume: %v\n", unmountErr)
		}
		return fmt.Errorf("failed to start container: %w", err)
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

	// Stop container
	if err := dockerManager.Stop(docker.DefaultContainerName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to stop container: %v\n", err)
	} else {
		fmt.Println("Container stopped.")
	}

	// Unmount volume
	if err := volumeManager.Unmount(mountPoint); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to unmount volume: %v\n", err)
	} else {
		fmt.Println("Volume unmounted.")
	}

	fmt.Println("Environment stopped.")

	if execErr != nil {
		return fmt.Errorf("shell exited with error: %w", execErr)
	}

	return nil
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop container and unmount volume",
		RunE:  runStop,
	}
}

func runStop(cmd *cobra.Command, args []string) error {
	dockerManager := docker.NewManager()

	// Get volume manager for unmount
	volumeManager, err := volume.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not create volume manager: %v\n", err)
	}

	// Stop container (symlink inside container is destroyed with it)
	fmt.Println("Stopping container...")
	if err := dockerManager.Stop(docker.DefaultContainerName); err != nil {
		fmt.Printf("Warning: Failed to stop container: %v\n", err)
	} else {
		fmt.Println("Container stopped.")
	}

	// Unmount volume
	if volumeManager != nil && volumeManager.IsMounted() {
		fmt.Println("Unmounting volume...")
		if err := volumeManager.Unmount(""); err != nil {
			fmt.Printf("Warning: Failed to unmount volume: %v\n", err)
		} else {
			fmt.Println("Volume unmounted.")
		}
	}

	fmt.Println("Environment stopped.")
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
		fmt.Println("Build it with: docker build -t portable-claude:latest .")
	}

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
