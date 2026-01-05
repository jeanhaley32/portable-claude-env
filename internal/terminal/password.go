package terminal

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/term"
)

// IsTerminal returns true if stdin is a terminal.
func IsTerminal() bool {
	return term.IsTerminal(int(syscall.Stdin))
}

// ReadPassword prompts for a password without echoing input.
func ReadPassword(prompt string) (string, error) {
	if !IsTerminal() {
		return "", fmt.Errorf("cannot read password: not a terminal")
	}

	fmt.Print(prompt)
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // newline after password entry
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	return string(password), nil
}

// ReadPasswordConfirm prompts for a password twice and verifies they match.
func ReadPasswordConfirm(prompt, confirmPrompt string) (string, error) {
	password, err := ReadPassword(prompt)
	if err != nil {
		return "", err
	}

	confirm, err := ReadPassword(confirmPrompt)
	if err != nil {
		return "", err
	}

	if password != confirm {
		return "", fmt.Errorf("passwords do not match")
	}

	return password, nil
}

// ReadPasswordFromEnv reads a password from an environment variable.
// Returns empty string if not set.
func ReadPasswordFromEnv(envVar string) string {
	return os.Getenv(envVar)
}
