package laravel

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

type RouteName struct {
	Name          string
	Path          string
	URI           string
	Range         textpos.Range
	TargetKind    string
	Target        string
	HandlerFQN    string
	HandlerMethod string
}

type RouteIndex struct {
	rootPath string

	writeMu sync.Mutex
	files   map[string]*routeFile

	mu     sync.RWMutex
	all    []RouteName
	names  []string
	byName map[string][]RouteName
}

func NewRouteIndex(rootPath string) *RouteIndex {
	return &RouteIndex{
		rootPath: rootPath,
		files:    make(map[string]*routeFile),
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

	var files []*routeFile
	err = filepath.Walk(routesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		files = append(files, parseRouteFile("file://"+path, string(content), idx.rootPath))
		return nil
	})
	idx.update(func(known map[string]*routeFile) {
		for _, f := range files {
			known[f.path] = f
		}
	})
	return err
}

func (idx *RouteIndex) IndexFile(uri, source string) {
	if !isLaravelRouteFile(uri) {
		return
	}
	f := parseRouteFile(uri, source, idx.rootPath)
	idx.update(func(known map[string]*routeFile) { known[f.path] = f })
}

func (idx *RouteIndex) RemoveFile(uri string) {
	if !isLaravelRouteFile(uri) {
		return
	}
	path := routeFilePath(uri)
	idx.update(func(known map[string]*routeFile) { delete(known, path) })
}

func (idx *RouteIndex) update(change func(map[string]*routeFile)) {
	idx.writeMu.Lock()
	defer idx.writeMu.Unlock()

	change(idx.files)
	all := readRouteFiles(idx.files, idx.rootPath)
	slices.SortFunc(all, compareRoutes)
	byName := make(map[string][]RouteName)
	names := make([]string, 0, len(all))
	for _, route := range all {
		if len(byName[route.Name]) == 0 {
			names = append(names, route.Name)
		}
		byName[route.Name] = append(byName[route.Name], route)
	}

	idx.mu.Lock()
	idx.all, idx.names, idx.byName = all, names, byName
	idx.mu.Unlock()
}

func compareRoutes(a, b RouteName) int {
	if c := strings.Compare(a.Name, b.Name); c != 0 {
		return c
	}
	if c := strings.Compare(a.URI, b.URI); c != 0 {
		return c
	}
	if a.Range.Start.Line != b.Range.Start.Line {
		return a.Range.Start.Line - b.Range.Start.Line
	}
	return a.Range.Start.Character - b.Range.Start.Character
}

func (idx *RouteIndex) Names() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return slices.Clone(idx.names)
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

func (idx *RouteIndex) All() []RouteName {
	if idx == nil {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return slices.Clone(idx.all)
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
