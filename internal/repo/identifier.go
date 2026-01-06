package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Pre-compiled regexes for sanitization (compiled once at package init)
var (
	pathSepRegex     = regexp.MustCompile(`[/:\\@\s]+`)
	unsafeCharRegex  = regexp.MustCompile(`[^a-zA-Z0-9._-]`)
	multiHyphenRegex = regexp.MustCompile(`-+`)
)

// Maximum length for repository identifiers
const maxIdentifierLength = 100

// ShortIDLength is the length of the short unique identifier (8 hex chars = 4 bytes)
const ShortIDLength = 8

// DefaultIdentifier implements Identifier using git commands.
type DefaultIdentifier struct{}

// NewIdentifier creates a new repository identifier.
func NewIdentifier() *DefaultIdentifier {
	return &DefaultIdentifier{}
}

func (d *DefaultIdentifier) GetRepoID(workspacePath string) (string, error) {
	// Try to get git remote URL
	cmd := exec.Command("git", "-C", workspacePath, "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		// Not a git repo or no remote, use directory name
		absPath, err := filepath.Abs(workspacePath)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path: %w", err)
		}
		return sanitizeName(filepath.Base(absPath)), nil
	}

	return normalizeRemoteURL(strings.TrimSpace(string(output))), nil
}

// GetShortID returns a short unique identifier for the workspace.
// This is a hash-based ID suitable for container names and mount paths.
// Format: 8 character hex string (e.g., "a1b2c3d4")
func (d *DefaultIdentifier) GetShortID(workspacePath string) (string, error) {
	repoID, err := d.GetRepoID(workspacePath)
	if err != nil {
		return "", err
	}

	// Hash the repoID to get a consistent short identifier
	hash := sha256.Sum256([]byte(repoID))
	return hex.EncodeToString(hash[:])[:ShortIDLength], nil
}

// GetContainerName returns the container name for the workspace.
// Format: "claude-<shortID>" (e.g., "claude-a1b2c3d4")
func (d *DefaultIdentifier) GetContainerName(workspacePath string) (string, error) {
	shortID, err := d.GetShortID(workspacePath)
	if err != nil {
		return "", err
	}
	return "claude-" + shortID, nil
}

func (d *DefaultIdentifier) GetWorkspaceRoot(path string) (string, error) {
	// Try to get git root
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		// Not a git repo, return the provided path
		return filepath.Abs(path)
	}

	return strings.TrimSpace(string(output)), nil
}

// normalizeRemoteURL converts a git remote URL to a filesystem-safe identifier.
// Examples:
//   - https://github.com/user/repo.git -> github.com-user-repo
//   - git@github.com:user/repo.git -> github.com-user-repo
func normalizeRemoteURL(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "git://")

	// Handle SSH format (git@github.com:user/repo.git)
	if strings.HasPrefix(url, "git@") {
		url = strings.TrimPrefix(url, "git@")
		url = strings.Replace(url, ":", "/", 1)
	}

	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	return sanitizeName(url)
}

// sanitizeName converts a string to a filesystem-safe name.
func sanitizeName(name string) string {
	// Replace path separators and special characters with hyphens
	name = pathSepRegex.ReplaceAllString(name, "-")

	// Remove any remaining unsafe characters
	name = unsafeCharRegex.ReplaceAllString(name, "")

	// Collapse multiple hyphens
	name = multiHyphenRegex.ReplaceAllString(name, "-")

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	// Limit length to avoid filesystem issues
	if len(name) > maxIdentifierLength {
		name = name[:maxIdentifierLength]
		// Ensure we don't end with a hyphen after truncation
		name = strings.TrimRight(name, "-")
	}

	// Return a default if name is empty
	if name == "" {
		name = "unknown-repo"
	}

	return name
}
