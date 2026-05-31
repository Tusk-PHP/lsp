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
	// Collect the header block lines; they are joined with hard line breaks
	// ("  \n") so each renders on its own line in markdown.
	var lines []string

	// Title: [name](url) — constraint (installed: version)
	title := "[" + c.Name + "](" + c.ExternalURL + ")"
	if c.Constraint != "" {
		title += " — " + c.Constraint
	}
	if c.InstalledVersion != "" {
		title += " (installed: " + c.InstalledVersion + ")"
	}
	lines = append(lines, title)

	// PHP requirement line.
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

	// License line.
	if len(c.License) > 0 {
		lines = append(lines, "License: "+strings.Join(c.License, ", "))
	}

	// Join with hard line breaks so each stat appears on its own line.
	out := strings.Join(lines, "  \n")

	// Description as a separate paragraph below the header block.
	if c.Description != "" {
		out += "\n\n" + c.Description
	}

	return out
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
