package terminal

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// PasswordEnvVar is the environment variable name for the volume password.
const PasswordEnvVar = "CAPSULE_PASSWORD"

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

// Reader returns an io.Reader for the password bytes.
// This avoids creating a string copy of the password.
func (s *SecurePassword) Reader() io.Reader {
	if s.data == nil {
		return bytes.NewReader(nil)
	}
	return bytes.NewReader(s.data)
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
// Deprecated: Use ReadPasswordFromStdinSecure for better memory safety.
func ReadPasswordFromStdin() (string, error) {
	password, err := ReadPasswordFromStdinSecure()
	if err != nil {
		return "", err
	}
	return password.String(), nil
}

// ReadPasswordFromStdinSecure reads a password from stdin and returns a SecurePassword.
func ReadPasswordFromStdinSecure() (*SecurePassword, error) {
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read password from stdin: %w", err)
	}
	trimmed := strings.TrimSuffix(password, "\n")
	return &SecurePassword{data: []byte(trimmed)}, nil
}

// ReadPasswordFromEnv reads the password from CAPSULE_PASSWORD environment variable.
// Returns empty string if not set.
// Deprecated: Use ReadPasswordFromEnvSecure for better memory safety.
func ReadPasswordFromEnv() string {
	return os.Getenv(PasswordEnvVar)
}

// ReadPasswordFromEnvSecure reads the password from CAPSULE_PASSWORD and returns a SecurePassword.
// Returns nil if not set.
func ReadPasswordFromEnvSecure() *SecurePassword {
	env := os.Getenv(PasswordEnvVar)
	if env == "" {
		return nil
	}
	return &SecurePassword{data: []byte(env)}
}

// ReadPasswordMultiSource attempts to read password from multiple sources in order:
// 1. If useStdin is true, read from stdin (for piped input)
// 2. Check CAPSULE_PASSWORD environment variable
// 3. Fall back to interactive terminal prompt
// Deprecated: Use ReadPasswordMultiSourceSecure for better memory safety.
func ReadPasswordMultiSource(useStdin bool, prompt string) (string, error) {
	password, err := ReadPasswordMultiSourceSecure(useStdin, prompt)
	if err != nil {
		return "", err
	}
	return password.String(), nil
}

// ReadPasswordMultiSourceSecure attempts to read password from multiple sources.
// The caller must call Clear() on the returned password when done.
func ReadPasswordMultiSourceSecure(useStdin bool, prompt string) (*SecurePassword, error) {
	// Option 1: Read from stdin if flag is set
	if useStdin {
		return ReadPasswordFromStdinSecure()
	}

	// Option 2: Check environment variable
	if envPassword := ReadPasswordFromEnvSecure(); envPassword != nil {
		return envPassword, nil
	}

	// Option 3: Interactive terminal prompt
	return ReadPasswordSecure(prompt)
}
