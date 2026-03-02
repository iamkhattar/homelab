package ui

import (
	"fmt"
	"strings"

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
	Cyan      = lipgloss.Color("#06B6D4")
	White     = lipgloss.Color("#E5E7EB")

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

	HighlightStyle = lipgloss.NewStyle().
			Foreground(Cyan)

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

// Header prints a prominent section header with a divider.
func Header(name string) {
	fmt.Println()
	fmt.Printf("🏠 %s\n", Title.Render(name))
	fmt.Println(SubtleStyle.Render(strings.Repeat("─", 40)))
}

// Step prints a step indicator for multi-step operations.
func Step(name string) {
	fmt.Printf("🔧 %s\n",
		lipgloss.NewStyle().Bold(true).Foreground(White).Render(name))
}

// StepDone prints a success indicator.
func StepDone(name string) {
	fmt.Printf("✅ %s\n", SuccessStyle.Render(name))
}

// StepFail prints a failure indicator.
func StepFail(name string) {
	fmt.Printf("❌ %s\n", ErrorStyle.Render(name))
}

// StepWarn prints a warning indicator.
func StepWarn(name string) {
	fmt.Printf("⚠️  %s\n", WarningStyle.Render(name))
}

// KeyValue prints a key-value pair with styled formatting.
func KeyValue(key, value string) {
	fmt.Printf("  %s %s\n", SubtleStyle.Render(key+":"), HighlightStyle.Render(value))
}

// Info prints a dimmed informational message.
func Info(msg string) {
	fmt.Printf("  💡 %s\n", SubtleStyle.Render(msg))
}
