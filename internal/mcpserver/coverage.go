package mcpserver

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tusk-PHP/lsp/internal/coverage"
)

// maxLeastCoveredFiles limits how many files php_coverage lists for a project.
const maxLeastCoveredFiles = 20

type PhpCoverageInput struct {
	Path string `json:"path,omitempty" jsonschema:"optional workspace-relative or absolute PHP file path; omit for project totals"`
}

type CoverageMethodRecord struct {
	Name       string  `json:"name"`
	Line       int     `json:"line"`
	Covered    int     `json:"coveredLines"`
	Executable int     `json:"executableLines"`
	CRAP       float64 `json:"crap,omitempty"`
}

type CoverageFileRecord struct {
	Path            string                 `json:"path"`
	CoveredLines    int                    `json:"coveredLines"`
	ExecutableLines int                    `json:"executableLines"`
	Percent         float64                `json:"percent"`
	UncoveredLines  []string               `json:"uncoveredLines,omitempty"`
	UntestedMethods []CoverageMethodRecord `json:"untestedMethods,omitempty"`
	PartialMethods  []CoverageMethodRecord `json:"partialMethods,omitempty"`
}

type PhpCoverageOutput struct {
	ReportFound bool   `json:"reportFound"`
	ReportPath  string `json:"reportPath,omitempty"`
	GeneratedAt string `json:"generatedAt,omitempty"`
	// Stale means the file changed after the report, so its line numbers may be off.
	Stale           bool                 `json:"stale,omitempty"`
	Note            string               `json:"note,omitempty"`
	File            *CoverageFileRecord  `json:"file,omitempty"`
	CoveredLines    int                  `json:"coveredLines,omitempty"`
	ExecutableLines int                  `json:"executableLines,omitempty"`
	Percent         float64              `json:"percent,omitempty"`
	LeastCovered    []CoverageFileRecord `json:"leastCovered,omitempty"`
}

func (s *Service) phpCoverage(path string) PhpCoverageOutput {
	s.coverage.Refresh()

	reportPath, generated, ok := s.coverage.ReportInfo()
	if !ok {
		return PhpCoverageOutput{Note: "No Clover report found. Run the tests with --coverage-clover clover.xml, or set coverageReportPath."}
	}
	out := PhpCoverageOutput{
		ReportFound: true,
		ReportPath:  normalizedRelativePath(s.RootPath, reportPath),
		GeneratedAt: generated.UTC().Format(time.RFC3339),
	}

	if strings.TrimSpace(path) != "" {
		absPath := path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(s.RootPath, path)
		}
		file, stale := s.coverage.File(absPath)
		if file == nil {
			out.Note = "This file isn't in the report, so its coverage is unknown. That doesn't mean it's untested."
			return out
		}
		record := coverageFileRecord(file, s.coverage.Relative(file))
		record.UncoveredLines = lineRanges(file.Uncovered)
		for _, m := range file.Methods {
			if m.Executable == 0 || m.Covered == m.Executable {
				continue
			}
			method := CoverageMethodRecord{Name: m.Name, Line: m.Line, Covered: m.Covered, Executable: m.Executable, CRAP: m.CRAP}
			if m.Covered == 0 {
				record.UntestedMethods = append(record.UntestedMethods, method)
			} else {
				record.PartialMethods = append(record.PartialMethods, method)
			}
		}
		out.File = &record
		out.Stale = stale
		return out
	}

	var records []CoverageFileRecord
	for _, file := range s.coverage.Files() {
		out.CoveredLines += file.Covered()
		out.ExecutableLines += len(file.Lines)
		if len(file.Lines) > 0 {
			records = append(records, coverageFileRecord(file, s.coverage.Relative(file)))
		}
	}
	out.Percent = percent(out.CoveredLines, out.ExecutableLines)
	sort.SliceStable(records, func(i, j int) bool { return records[i].Percent < records[j].Percent })
	if len(records) > maxLeastCoveredFiles {
		records = records[:maxLeastCoveredFiles]
	}
	out.LeastCovered = records
	return out
}

func coverageFileRecord(file *coverage.File, path string) CoverageFileRecord {
	return CoverageFileRecord{
		Path:            path,
		CoveredLines:    file.Covered(),
		ExecutableLines: len(file.Lines),
		Percent:         percent(file.Covered(), len(file.Lines)),
	}
}

func percent(covered, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(covered*10000/total) / 100
}

// lineRanges compresses sorted line numbers: [3 4 5 9] becomes ["3-5", "9"].
func lineRanges(lines []int) []string {
	var ranges []string
	for i := 0; i < len(lines); {
		j := i
		for j+1 < len(lines) && lines[j+1] == lines[j]+1 {
			j++
		}
		if i == j {
			ranges = append(ranges, fmt.Sprintf("%d", lines[i]))
		} else {
			ranges = append(ranges, fmt.Sprintf("%d-%d", lines[i], lines[j]))
		}
		i = j + 1
	}
	return ranges
}
