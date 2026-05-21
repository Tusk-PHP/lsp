package laravel

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

type RouteName struct {
	Name  string
	URI   string
	Range protocol.Range
}

type RouteIndex struct {
	rootPath string

	mu     sync.RWMutex
	byURI  map[string][]RouteName
	byName map[string][]RouteName
}

func NewRouteIndex(rootPath string) *RouteIndex {
	return &RouteIndex{
		rootPath: rootPath,
		byURI:    make(map[string][]RouteName),
		byName:   make(map[string][]RouteName),
	}
}

func (idx *RouteIndex) ScanWorkspace() error {
	routesDir := filepath.Join(idx.rootPath, "routes")
	info, err := os.Stat(routesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	return filepath.Walk(routesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		idx.IndexFile("file://"+path, string(content))
		return nil
	})
}

func (idx *RouteIndex) IndexFile(uri, source string) {
	if !isLaravelRouteFile(uri) {
		return
	}

	routes := parseRouteNames(uri, source)

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if prev := idx.byURI[uri]; len(prev) > 0 {
		for _, route := range prev {
			idx.removeRouteLocked(route)
		}
	}

	idx.byURI[uri] = routes
	for _, route := range routes {
		idx.byName[route.Name] = append(idx.byName[route.Name], route)
		idx.sortRouteDefsLocked(route.Name)
	}
}

func (idx *RouteIndex) Names() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	names := make([]string, 0, len(idx.byName))
	for name, entries := range idx.byName {
		if name != "" && len(entries) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (idx *RouteIndex) Find(name string) *RouteName {
	if idx == nil || name == "" {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entries := idx.byName[name]
	if len(entries) == 0 {
		return nil
	}
	route := entries[0]
	return &route
}

func (idx *RouteIndex) removeRouteLocked(route RouteName) {
	entries := idx.byName[route.Name]
	if len(entries) == 0 {
		return
	}

	filtered := entries[:0]
	for _, entry := range entries {
		if entry.URI == route.URI &&
			entry.Range.Start == route.Range.Start &&
			entry.Range.End == route.Range.End {
			continue
		}
		filtered = append(filtered, entry)
	}

	if len(filtered) == 0 {
		delete(idx.byName, route.Name)
		return
	}
	idx.byName[route.Name] = filtered
}

func (idx *RouteIndex) sortRouteDefsLocked(name string) {
	sort.Slice(idx.byName[name], func(i, j int) bool {
		a := idx.byName[name][i]
		b := idx.byName[name][j]
		if a.URI != b.URI {
			return a.URI < b.URI
		}
		if a.Range.Start.Line != b.Range.Start.Line {
			return a.Range.Start.Line < b.Range.Start.Line
		}
		return a.Range.Start.Character < b.Range.Start.Character
	})
}

func parseRouteNames(uri, source string) []RouteName {
	result := parser.New().Parse(source)
	var routes []RouteName

	for i := 0; i < len(result.Tokens); i++ {
		if result.Tokens[i].Kind != parser.TokenArrow {
			continue
		}

		nameTok := nextSignificantToken(result.Tokens, i+1)
		if nameTok < 0 || result.Tokens[nameTok].Kind != parser.TokenIdentifier || result.Tokens[nameTok].Value != "name" {
			continue
		}

		openTok := nextSignificantToken(result.Tokens, nameTok+1)
		if openTok < 0 || result.Tokens[openTok].Kind != parser.TokenOpenParen {
			continue
		}

		valueTok := nextSignificantToken(result.Tokens, openTok+1)
		if valueTok < 0 || result.Tokens[valueTok].Kind != parser.TokenStringLiteral {
			continue
		}

		raw := result.Tokens[valueTok].Value
		name := trimQuotedLiteral(raw)
		if name == "" {
			continue
		}

		startCol := result.Tokens[valueTok].Column
		endCol := startCol + len(raw)
		if len(raw) >= 2 {
			startCol++
			endCol--
		}

		routes = append(routes, RouteName{
			Name: name,
			URI:  uri,
			Range: protocol.Range{
				Start: protocol.Position{Line: result.Tokens[valueTok].Line, Character: startCol},
				End:   protocol.Position{Line: result.Tokens[valueTok].Line, Character: endCol},
			},
		})
	}

	return routes
}

func nextSignificantToken(tokens []parser.Token, start int) int {
	for i := start; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			continue
		default:
			return i
		}
	}
	return -1
}

func trimQuotedLiteral(raw string) string {
	if len(raw) >= 2 && (raw[0] == '\'' || raw[0] == '"') && raw[len(raw)-1] == raw[0] {
		return raw[1 : len(raw)-1]
	}
	return raw
}

func isLaravelRouteFile(uri string) bool {
	path := filepath.ToSlash(symbols.URIToPath(uri))
	return strings.HasSuffix(path, ".php") && strings.Contains(path, "/routes/")
}

var routeCallPatterns = []string{
	"route(",
	"to_route(",
	"->route(",
	"::route(",
	"->routeIs(",
}

func ExtractRouteNameArgContext(trimmed string) (string, string, bool) {
	bestIdx := -1
	bestPattern := ""
	for _, pattern := range routeCallPatterns {
		idx := strings.LastIndex(trimmed, pattern)
		if idx > bestIdx {
			bestIdx = idx
			bestPattern = pattern
		}
	}
	if bestIdx < 0 {
		return "", "", false
	}

	after := trimmed[bestIdx+len(bestPattern):]
	if strings.Contains(after, ")") {
		return "", "", false
	}

	quote := ""
	if len(after) > 0 && (after[0] == '\'' || after[0] == '"') {
		quote = string(after[0])
		after = after[1:]
	}

	return after, quote, true
}

func FindRouteNameReference(line string, character int) (string, bool) {
	if character < 0 || len(line) == 0 {
		return "", false
	}
	if character >= len(line) {
		character = len(line) - 1
	}
	if character < 0 {
		return "", false
	}

	prefix := strings.TrimSpace(line[:character+1])
	if _, _, ok := ExtractRouteNameArgContext(prefix); !ok {
		return "", false
	}

	start, end, ok := enclosingQuotedString(line, character)
	if !ok {
		return "", false
	}

	if _, _, ok := ExtractRouteNameArgContext(strings.TrimSpace(line[:start+1])); !ok {
		return "", false
	}

	if end-start < 2 {
		return "", false
	}
	return line[start+1 : end], true
}

func enclosingQuotedString(line string, character int) (int, int, bool) {
	for start := character; start >= 0; start-- {
		if !isQuote(line[start]) || isEscaped(line, start) {
			continue
		}
		quote := line[start]
		for end := start + 1; end < len(line); end++ {
			if line[end] != quote || isEscaped(line, end) {
				continue
			}
			if character > start && character < end {
				return start, end, true
			}
			break
		}
	}
	return 0, 0, false
}

func isQuote(ch byte) bool {
	return ch == '\'' || ch == '"'
}

func isEscaped(s string, idx int) bool {
	backslashes := 0
	for i := idx - 1; i >= 0 && s[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}
