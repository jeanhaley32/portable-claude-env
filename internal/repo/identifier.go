package repo

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

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
	re := regexp.MustCompile(`[/:\\@\s]+`)
	name = re.ReplaceAllString(name, "-")

	// Remove any remaining unsafe characters
	re = regexp.MustCompile(`[^a-zA-Z0-9._-]`)
	name = re.ReplaceAllString(name, "")

	// Collapse multiple hyphens
	re = regexp.MustCompile(`-+`)
	name = re.ReplaceAllString(name, "-")

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	return name
}
