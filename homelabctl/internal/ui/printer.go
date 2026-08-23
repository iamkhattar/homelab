package ui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
)

// Printer is the single presentation layer for homelabctl-owned output.
// Lip Gloss automatically removes ANSI styling for redirected output and
// respects NO_COLOR, CLICOLOR, and CLICOLOR_FORCE.
type Printer struct {
	out io.Writer
}

type Status int

const (
	Running Status = iota
	Success
	Failure
	Warning
	Skipped
	Info
)

var (
	brandStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22C55E"))
	failureStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EF4444"))
	warningStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
	infoStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#38BDF8"))
	mutedStyle   = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#94A3B8"))
	keyStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A78BFA"))
)

func New(out io.Writer) *Printer {
	return &Printer{out: out}
}

func (p *Printer) Printf(format string, args ...any) {
	_, _ = lipgloss.Fprintf(p.out, format, args...)
}

func (p *Printer) Heading(title string) {
	p.Printf("%s %s\n", brandStyle.Render("◆"), brandStyle.Render(title))
}

func (p *Printer) Status(status Status, label, message string) {
	symbol, style := statusPresentation(status)
	label = strings.ToUpper(strings.TrimSpace(label))
	paddedLabel := fmt.Sprintf("%-6s", label)
	p.Printf("%s %s%s\n", style.Render(symbol), style.Render(paddedLabel), message)
}

func (p *Printer) KeyValue(key, value string) {
	p.Printf("  %s  %s\n", keyStyle.Render(fmt.Sprintf("%-16s", key)), value)
}

func (p *Printer) Success(message string) {
	p.Status(Success, "done", message)
}

func (p *Printer) Info(message string) {
	p.Status(Info, "info", message)
}

func (p *Printer) Warning(message string) {
	p.Status(Warning, "warn", message)
}

func (p *Printer) Error(message string) {
	p.Status(Failure, "error", message)
}

func (p *Printer) Command(directory, command string) {
	prefix := "+"
	if directory != "" {
		p.Printf("%s %s %s\n", infoStyle.Render(prefix), mutedStyle.Render("("+directory+")"), command)
		return
	}
	p.Printf("%s %s\n", infoStyle.Render(prefix), command)
}

func statusPresentation(status Status) (string, lipgloss.Style) {
	switch status {
	case Running:
		return "◆", infoStyle
	case Success:
		return "✓", successStyle
	case Failure:
		return "✗", failureStyle
	case Warning:
		return "!", warningStyle
	case Skipped:
		return "–", mutedStyle
	case Info:
		return "•", infoStyle
	default:
		return "•", mutedStyle
	}
}
