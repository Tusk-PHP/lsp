package cardhover

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/composer"
)

// card is the structured data for one hover. render() turns it into
// markdown. Keeping the data structured (rather than rendering inline)
// makes the formatting trivially testable.
type card struct {
	Name             string
	Constraint       string // raw value from composer.json (e.g. "^10.0")
	ExternalURL      string
	Description      string
	InstalledVersion string
	PHPRequire       string
	License          []string
	Homepage         string
	SourceURL        string
	ProjectPHPVer    string // resolved workspace PHP "MAJOR.MINOR"
}

func (c card) render() string {
	var b strings.Builder

	// Title: linked name + constraint.
	b.WriteString("[")
	b.WriteString(c.Name)
	b.WriteString("]")
	b.WriteString("(")
	b.WriteString(c.ExternalURL)
	b.WriteString(")")
	if c.Constraint != "" {
		b.WriteString(" — `")
		b.WriteString(c.Constraint)
		b.WriteString("`")
	}
	b.WriteString("\n")

	if c.Description != "" {
		b.WriteString("\n")
		b.WriteString(c.Description)
		b.WriteString("\n")
	}

	// Stats block: every present line on its own row.
	stats := c.statsLines()
	if len(stats) > 0 {
		b.WriteString("\n")
		for _, line := range stats {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func (c card) statsLines() []string {
	var lines []string

	if c.PHPRequire != "" {
		glyph := compatGlyph(c.ProjectPHPVer, c.PHPRequire)
		line := "Requires PHP " + c.PHPRequire
		if glyph != "" {
			line += " " + glyph
			if c.ProjectPHPVer != "" {
				line += " (project targets " + c.ProjectPHPVer + ")"
			}
		}
		lines = append(lines, line)
	}

	if c.InstalledVersion != "" {
		lines = append(lines, "Installed: `"+c.InstalledVersion+"`")
	}

	if len(c.License) > 0 {
		lines = append(lines, "License: "+strings.Join(c.License, ", "))
	}

	if c.SourceURL != "" {
		lines = append(lines, "Source: "+c.SourceURL)
	}

	if c.Homepage != "" {
		lines = append(lines, "Homepage: "+c.Homepage)
	}

	return lines
}

// compatGlyph picks a small unicode marker indicating whether the project
// PHP version satisfies the package's PHP constraint. Returns "" when the
// comparison cannot be made — the caller then renders the constraint
// alone, without a glyph.
func compatGlyph(projectVer, pkgConstraint string) string {
	switch composer.CompareConstraint(projectVer, pkgConstraint) {
	case +1:
		return "✓"
	case -1:
		return "✗"
	}
	return ""
}
