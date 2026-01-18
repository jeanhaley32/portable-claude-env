package terminal

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// PromptChoice displays a numbered menu and returns the selected index (0-based).
// The prompt includes a default option that is selected if the user presses Enter.
func PromptChoice(question string, options []string, defaultIndex int) (int, error) {
	if !IsTerminal() {
		return defaultIndex, nil
	}

	fmt.Println(question)
	for i, opt := range options {
		fmt.Printf("  %d. %s\n", i+1, opt)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Selection [%d]: ", defaultIndex+1)
		input, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)

		// Default selection
		if input == "" {
			return defaultIndex, nil
		}

		// Parse selection
		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(options) {
			fmt.Printf("Please enter a number between 1 and %d\n", len(options))
			continue
		}

		return num - 1, nil
	}
}

// PromptIntWithDefault prompts for an integer with a default value.
// Returns the default if the user presses Enter without input.
func PromptIntWithDefault(question string, defaultVal int) (int, error) {
	if !IsTerminal() {
		return defaultVal, nil
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [%d]: ", question, defaultVal)
		input, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)

		// Default value
		if input == "" {
			return defaultVal, nil
		}

		// Parse input
		num, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println("Please enter a valid number")
			continue
		}

		return num, nil
	}
}
