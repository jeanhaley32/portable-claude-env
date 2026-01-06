package terminal

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// PasswordEnvVar is the environment variable name for the volume password.
const PasswordEnvVar = "CLAUDE_ENV_PASSWORD"

// SecurePassword wraps a password with the ability to clear it from memory.
type SecurePassword struct {
	data []byte
}

// String returns the password as a string.
func (s *SecurePassword) String() string {
	if s.data == nil {
		return ""
	}
	return string(s.data)
}

// Clear zeros out the password data in memory.
// Should be called when the password is no longer needed.
func (s *SecurePassword) Clear() {
	if s.data != nil {
		for i := range s.data {
			s.data[i] = 0
		}
		s.data = nil
	}
}

// Len returns the length of the password.
func (s *SecurePassword) Len() int {
	return len(s.data)
}

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

// ReadPasswordSecure prompts for a password and returns a SecurePassword
// that can be cleared from memory when no longer needed.
func ReadPasswordSecure(prompt string) (*SecurePassword, error) {
	if !IsTerminal() {
		return nil, fmt.Errorf("cannot read password: not a terminal")
	}

	fmt.Print(prompt)
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // newline after password entry
	if err != nil {
		return nil, fmt.Errorf("failed to read password: %w", err)
	}

	return &SecurePassword{data: password}, nil
}

// ReadPasswordConfirm prompts for a password twice and verifies they match.
// Deprecated: Use ReadPasswordConfirmSecure for better memory safety.
func ReadPasswordConfirm(prompt, confirmPrompt string) (string, error) {
	password, err := ReadPasswordConfirmSecure(prompt, confirmPrompt)
	if err != nil {
		return "", err
	}
	// Note: caller should clear password when done
	return password.String(), nil
}

// ReadPasswordConfirmSecure prompts for a password twice, verifies they match,
// and returns a SecurePassword that can be cleared from memory.
func ReadPasswordConfirmSecure(prompt, confirmPrompt string) (*SecurePassword, error) {
	password, err := ReadPasswordSecure(prompt)
	if err != nil {
		return nil, err
	}

	confirm, err := ReadPasswordSecure(confirmPrompt)
	if err != nil {
		password.Clear()
		return nil, err
	}

	if password.String() != confirm.String() {
		password.Clear()
		confirm.Clear()
		return nil, fmt.Errorf("passwords do not match")
	}

	confirm.Clear()
	return password, nil
}

// ReadPasswordFromStdin reads a password from stdin (for piped input).
// Use this when --password-stdin flag is provided.
func ReadPasswordFromStdin() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read password from stdin: %w", err)
	}
	return strings.TrimSuffix(password, "\n"), nil
}

// ReadPasswordFromEnv reads the password from CLAUDE_ENV_PASSWORD environment variable.
// Returns empty string if not set.
func ReadPasswordFromEnv() string {
	return os.Getenv(PasswordEnvVar)
}

// ReadPasswordMultiSource attempts to read password from multiple sources in order:
// 1. If useStdin is true, read from stdin (for piped input)
// 2. Check CLAUDE_ENV_PASSWORD environment variable
// 3. Fall back to interactive terminal prompt
func ReadPasswordMultiSource(useStdin bool, prompt string) (string, error) {
	// Option 1: Read from stdin if flag is set
	if useStdin {
		return ReadPasswordFromStdin()
	}

	// Option 2: Check environment variable
	if envPassword := ReadPasswordFromEnv(); envPassword != "" {
		return envPassword, nil
	}

	// Option 3: Interactive terminal prompt
	return ReadPassword(prompt)
}
