package terminal

import (
	"fmt"
	"syscall"

	"golang.org/x/term"
)

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
