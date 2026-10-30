package lsp

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tusk-PHP/lsp/internal/coverage"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// handleCoverageReport answers tusk/coverageReport with the loaded report's files
// and their totals. It reads from disk, so it doesn't hold up other requests.
func (s *Server) handleCoverageReport(msg *jsonRPCMessage) {
	if s.coverage == nil {
		s.sendResponse(msg.ID, nil)
		return
	}
	s.goSafe("tusk/coverageReport", func() {
		s.coverage.Refresh()
		s.sendResponse(msg.ID, s.coverageReport())
	})
}

func (s *Server) coverageReport() *protocol.CoverageReport {
	reportPath, generated, ok := s.coverage.ReportInfo()
	if !ok {
		return nil
	}
	report := &protocol.CoverageReport{
		Path:      reportPath,
		Generated: generated.UTC().Format(time.RFC3339),
		Files:     []protocol.CoverageFileSummary{},
	}
	for _, f := range s.coverage.Files() {
		local := s.coverage.LocalPath(f)
		if local == "" {
			continue
		}
		file, stale := s.coverage.File(local)
		if file == nil {
			continue
		}
		summary := protocol.CoverageFileSummary{
			URI:             pathToFileURI(local),
			Stale:           stale,
			CoveredLines:    file.Covered(),
			ExecutableLines: len(file.Lines),
		}
		for _, m := range file.Methods {
			if m.Executable == 0 {
				continue
			}
			summary.Methods++
			if m.Covered > 0 {
				summary.CoveredMethods++
			}
		}
		report.Files = append(report.Files, summary)
	}
	return report
}

// handleFileCoverage answers tusk/fileCoverage with a file's lines and methods.
// Like hover, it never reads the report on this path, and answers null when
// coverage is unknown so a missing file never reads as untested.
func (s *Server) handleFileCoverage(msg *jsonRPCMessage) {
	var params protocol.FileCoverageParams
	if json.Unmarshal(msg.Params, &params) != nil {
		s.sendError(msg.ID, -32602, "Invalid params")
		return
	}
	if s.coverage == nil {
		s.sendResponse(msg.ID, nil)
		return
	}
	s.coverage.RefreshAsync()
	file, stale := s.coverage.File(symbols.URIToPath(params.TextDocument.URI))
	if file == nil {
		s.sendResponse(msg.ID, nil)
		return
	}
	s.sendResponse(msg.ID, fileCoverageResult(params.TextDocument.URI, file, stale))
}

func fileCoverageResult(uri string, file *coverage.File, stale bool) *protocol.FileCoverage {
	result := &protocol.FileCoverage{
		URI:     uri,
		Stale:   stale,
		Lines:   make([]protocol.LineCoverage, 0, len(file.Lines)),
		Methods: make([]protocol.MethodCoverage, 0, len(file.Methods)),
	}
	for line, hits := range file.Lines {
		result.Lines = append(result.Lines, protocol.LineCoverage{Line: line - 1, Hits: hits})
	}
	sort.Slice(result.Lines, func(i, j int) bool { return result.Lines[i].Line < result.Lines[j].Line })
	for _, m := range file.Methods {
		// With no executable lines there's nothing a test could have reached, so
		// listing it would read as untested.
		if m.Executable == 0 {
			continue
		}
		hits := m.Hits
		if hits == 0 {
			hits = m.Covered
		}
		result.Methods = append(result.Methods, protocol.MethodCoverage{
			Name:       m.Name,
			Line:       m.Line - 1,
			EndLine:    m.EndLine - 1,
			Hits:       hits,
			Covered:    m.Covered,
			Executable: m.Executable,
		})
	}
	return result
}

func pathToFileURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows drive paths: C:/x → /C:/x
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
