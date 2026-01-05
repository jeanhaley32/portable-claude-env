package config

// Config holds the application configuration.
type Config struct {
	// VolumePath is the path to the encrypted volume file.
	VolumePath string

	// WorkspacePath is the path to the current workspace/project.
	WorkspacePath string

	// ContainerName is the name of the Docker container.
	ContainerName string

	// ImageName is the name of the Docker image.
	ImageName string
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		ContainerName: "portable-claude",
		ImageName:     "portable-claude:latest",
	}
}

// VolumeStructure defines the directory structure inside the encrypted volume.
var VolumeStructure = []string{
	"auth",           // API keys, authentication tokens
	"config",         // User preferences, Claude Code settings
	"claude-context", // .claude conversation history
	"bootstrap",      // Templates and starting files
	"repos",          // Per-repository documentation and context
	"home",           // User home directory (persists Claude credentials, shell history, etc.)
}
