// Package wizard provides an interactive manifest builder wizard.
package wizard

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Prompt writes "label [defaultVal]: " to w and reads one line from r.
// Returns defaultVal when the line is empty or r is not a terminal (stdin
// redirect / CI). Never blocks on non-interactive input.
func Prompt(w io.Writer, r io.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Fprintf(w, "%s [%s]: ", label, defaultVal)
	} else {
		fmt.Fprintf(w, "%s: ", label)
	}
	// Check if r is os.Stdin and whether it is a terminal.
	if f, ok := r.(*os.File); ok && !isTerminal(f) {
		fmt.Fprintln(w, defaultVal)
		return defaultVal
	}
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		fmt.Fprintln(w, defaultVal)
		return defaultVal
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return defaultVal
	}
	return line
}

// PromptBool prompts with a [y/n] default and returns a bool.
func PromptBool(w io.Writer, r io.Reader, label string, defaultVal bool) bool {
	def := "n"
	if defaultVal {
		def = "y"
	}
	resp := Prompt(w, r, label+" (y/n)", def)
	return strings.ToLower(resp) == "y" || strings.ToLower(resp) == "yes"
}

// PromptSelect displays a numbered list of options and returns the selected
// value. If the user enters a number, the corresponding option is returned.
// If the user enters a string matching an option, that option is returned.
// Empty input returns options[defaultIdx].
func PromptSelect(w io.Writer, r io.Reader, label string, options []string, defaultIdx int) string {
	fmt.Fprintf(w, "\n%s:\n", label)
	for i, opt := range options {
		marker := " "
		if i == defaultIdx {
			marker = "*"
		}
		fmt.Fprintf(w, "  %s %d. %s\n", marker, i+1, opt)
	}
	def := ""
	if defaultIdx >= 0 && defaultIdx < len(options) {
		def = options[defaultIdx]
	}
	resp := Prompt(w, r, "Select (number or name)", def)
	// Try numeric selection.
	var n int
	if _, err := fmt.Sscanf(resp, "%d", &n); err == nil && n >= 1 && n <= len(options) {
		return options[n-1]
	}
	// Try name match.
	for _, opt := range options {
		if strings.EqualFold(resp, opt) {
			return opt
		}
	}
	return def
}

// isTerminal reports whether f is connected to a terminal.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
