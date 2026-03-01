package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	Primary   = lipgloss.Color("#7C3AED")
	Success   = lipgloss.Color("#10B981")
	Warning   = lipgloss.Color("#F59E0B")
	Error     = lipgloss.Color("#EF4444")
	Subtle    = lipgloss.Color("#6B7280")
	Highlight = lipgloss.Color("#38BDF8")

	// Text styles
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(Primary)

	Heading = lipgloss.NewStyle().
		Bold(true).
		Foreground(Highlight)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(Success)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Error)

	WarningStyle = lipgloss.NewStyle().
			Foreground(Warning)

	SubtleStyle = lipgloss.NewStyle().
			Foreground(Subtle)

	Bold = lipgloss.NewStyle().
		Bold(true)

	// Status indicators
	StatusRunning = SuccessStyle.Render("●")
	StatusStopped = ErrorStyle.Render("●")
	StatusPending = WarningStyle.Render("●")

	// Borders
	Section = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(Subtle).
		Padding(0, 1)
)

// Step prints a step indicator for multi-step operations.
func Step(name string) {
	fmt.Printf("%s %s\n", Bold.Foreground(Primary).Render("==>"), Bold.Render(name))
}

// StepDone prints a success indicator.
func StepDone(name string) {
	fmt.Printf("%s %s\n", SuccessStyle.Render("✓"), name)
}

// StepFail prints a failure indicator.
func StepFail(name string) {
	fmt.Printf("%s %s\n", ErrorStyle.Render("✗"), name)
}

// KeyValue prints a key-value pair with styled formatting.
func KeyValue(key, value string) {
	fmt.Printf("  %s %s\n", SubtleStyle.Render(key+":"), value)
}
