package diagnostics

import (
	"context"
	"encoding/json"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

type pintRunner struct {
	externalTool
	cfgFile string
}

type pintOutput struct {
	Files []pintFile `json:"files"`
}

type pintFile struct {
	Name          string   `json:"name"`
	Diff          string   `json:"diff"`
	AppliedFixers []string `json:"appliedFixers"`
}

func newPintRunner(rootPath string, cfg *config.Config, logger *log.Logger) *pintRunner {
	r := &pintRunner{
		externalTool: newExternalTool("Pint", "pint", rootPath, cfg.PintPath, cfg.PintEnabled, cfg.PintTimeout(), logger),
		cfgFile:      cfg.PintConfig,
	}
	if r.cfgFile == "" {
		if candidate := filepath.Join(rootPath, "pint.json"); fileExists(candidate) {
			r.cfgFile = candidate
		}
	}
	if r.available() {
		logger.Printf("Pint: binary=%s config=%s", r.binPath, r.cfgFile)
	}
	return r
}

func (r *pintRunner) analyze(ctx context.Context, filePath string) []Diagnostic {
	args := []string{"--test", "--format=json"}
	if r.cfgFile != "" {
		args = append(args, "--config="+r.cfgFile)
	}
	output, ok := r.run(ctx, filePath, args...)
	if !ok {
		return nil
	}
	return r.parseOutput(output)
}

func (r *pintRunner) parseOutput(data []byte) []Diagnostic {
	jsonData := extractJSON(data)
	if jsonData == nil {
		return nil
	}

	var result pintOutput
	if err := json.Unmarshal(jsonData, &result); err != nil {
		r.logger.Printf("Pint JSON parse error: %v", err)
		return nil
	}

	var diags []Diagnostic
	for _, file := range result.Files {
		if file.Diff != "" {
			diags = append(diags, parseDiffDiagnostics(file.Diff, file.AppliedFixers)...)
		} else {
			// No diff available — report each fixer at file level
			for _, fixer := range file.AppliedFixers {
				diags = append(diags, Diagnostic{
					Range: textpos.Range{
						Start: textpos.Position{Line: 0, Character: 0},
						End:   textpos.Position{Line: 0, Character: 0},
					},
					Severity: rule.SeverityWarning,
					Source:   string(rule.SourcePint),
					Message:  formatFixerName(fixer),
					Code:     fixer,
				})
			}
		}
	}

	return diags
}

var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+\d+(?:,\d+)? @@`)

// parseDiffDiagnostics extracts line-precise diagnostics from a unified diff.
// Removed lines (starting with '-') represent the original code that needs fixing.
func parseDiffDiagnostics(diff string, fixers []string) []Diagnostic {
	var diags []Diagnostic
	fixerDesc := rule.PintCode(fixers)
	message := formatFixerName(fixerDesc)

	lines := strings.Split(diff, "\n")
	origLine := 0
	reported := make(map[int]bool)

	for _, line := range lines {
		// Skip diff file headers
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			continue
		}
		if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
			origLine, _ = strconv.Atoi(m[1])
			continue
		}

		if len(line) == 0 {
			continue
		}

		switch line[0] {
		case '-':
			// This original line needs changes — report it
			lspLine := origLine - 1 // 1-based → 0-based
			if lspLine < 0 {
				lspLine = 0
			}
			if !reported[lspLine] {
				content := strings.TrimSpace(line[1:])
				col := 0
				if content != "" {
					// Find column of first non-whitespace in the removed line
					trimmed := strings.TrimLeft(line[1:], " \t")
					col = len(line[1:]) - len(trimmed)
				}
				diags = append(diags, Diagnostic{
					Range: textpos.Range{
						Start: textpos.Position{Line: lspLine, Character: col},
						End:   textpos.Position{Line: lspLine, Character: col + len(strings.TrimSpace(content))},
					},
					Severity: rule.SeverityWarning,
					Source:   string(rule.SourcePint),
					Message:  message,
					Code:     fixerDesc,
				})
				reported[lspLine] = true
			}
			origLine++
		case '+':
			// Added line — doesn't exist in original, don't increment
		default:
			// Context line
			origLine++
		}
	}

	return diags
}

func formatFixerName(name string) string {
	return "Code style: " + strings.ReplaceAll(name, "_", " ")
}
