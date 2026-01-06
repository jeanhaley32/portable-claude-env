package config

// VolumeStructure defines the directory structure inside the encrypted volume.
var VolumeStructure = []string{
	"auth",           // API keys, authentication tokens
	"config",         // User preferences, Claude Code settings
	"claude-context", // .claude conversation history
	"bootstrap",      // Templates and starting files
	"repos",          // Per-repository documentation and context
	"home",           // User home directory (persists Claude credentials, shell history, etc.)
	"home/.claude",   // Claude Code configuration directory
}
