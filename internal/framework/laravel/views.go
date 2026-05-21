package laravel

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type View struct {
	Name string
	URI  string
}

type Views struct {
	rootPath string

	mu     sync.RWMutex
	loaded bool
	views  []View
	byName map[string]View
}

func NewViews(rootPath string) *Views {
	return &Views{rootPath: rootPath}
}

func (v *Views) Search(prefix string) []View {
	if v == nil {
		return nil
	}
	v.load()

	v.mu.RLock()
	defer v.mu.RUnlock()

	lowerPrefix := strings.ToLower(prefix)
	wantsNamespace := strings.Contains(prefix, "::")
	var matches []View
	for _, view := range v.views {
		if wantsNamespace != strings.Contains(view.Name, "::") {
			continue
		}
		if lowerPrefix == "" || strings.HasPrefix(strings.ToLower(view.Name), lowerPrefix) {
			matches = append(matches, view)
		}
	}
	return matches
}

func (v *Views) Find(name string) *View {
	if v == nil || name == "" {
		return nil
	}
	v.load()

	v.mu.RLock()
	defer v.mu.RUnlock()

	if view, ok := v.byName[name]; ok {
		copy := view
		return &copy
	}
	return nil
}

func (v *Views) load() {
	v.mu.RLock()
	if v.loaded {
		v.mu.RUnlock()
		return
	}
	v.mu.RUnlock()

	viewsPath := filepath.Join(v.rootPath, "resources", "views")
	entries := collectViews(viewsPath)
	byName := make(map[string]View, len(entries))
	for _, view := range entries {
		byName[view.Name] = view
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.loaded {
		return
	}
	v.views = entries
	v.byName = byName
	v.loaded = true
}

func collectViews(root string) []View {
	var views []View
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		name, ok := viewNameFromPath(root, path)
		if !ok {
			return nil
		}
		uri := "file://" + filepath.ToSlash(path)
		views = append(views, View{Name: name, URI: uri})
		if alias := aliasViewName(name); alias != "" {
			views = append(views, View{Name: alias, URI: uri})
		}
		return nil
	})

	sort.Slice(views, func(i, j int) bool {
		if views[i].Name != views[j].Name {
			return views[i].Name < views[j].Name
		}
		return views[i].URI < views[j].URI
	})
	return views
}

func viewNameFromPath(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	switch {
	case strings.HasSuffix(rel, ".blade.php"):
		rel = strings.TrimSuffix(rel, ".blade.php")
	case strings.HasSuffix(rel, ".php"):
		rel = strings.TrimSuffix(rel, ".php")
	default:
		return "", false
	}
	if rel == "" {
		return "", false
	}
	return strings.ReplaceAll(rel, "/", "."), true
}

func aliasViewName(name string) string {
	parts := strings.Split(name, ".")
	if len(parts) < 2 || parts[0] == "" {
		return ""
	}
	return parts[0] + "::" + strings.Join(parts[1:], ".")
}

type ViewContext struct {
	Name    string
	Partial string
	Quote   string
}

func ExtractViewCompletionContext(prefix string) (*ViewContext, bool) {
	call, ok := findViewCall(prefix)
	if !ok {
		return nil, false
	}
	scan := prefix[call.argsStart:]
	argIndex := 0
	depth := 0
	var quote byte
	quoteStart := -1
	for i := 0; i < len(scan); i++ {
		ch := scan[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				quoteStart = -1
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
			quoteStart = i
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return nil, false
			}
			depth--
		case ',':
			if depth == 0 {
				argIndex++
			}
		}
	}
	if quote == 0 || quoteStart < 0 || argIndex != call.targetArg {
		return nil, false
	}
	return &ViewContext{
		Partial: scan[quoteStart+1:],
		Quote:   string(quote),
	}, true
}

func ExtractViewDefinitionContext(line string, char int) (*ViewContext, bool) {
	if char < 0 {
		return nil, false
	}
	if char > len(line) {
		char = len(line)
	}
	ctx, ok := ExtractViewCompletionContext(line[:char])
	if !ok {
		return nil, false
	}
	start := char
	for start > 0 && line[start-1] != ctx.Quote[0] {
		start--
	}
	if start == 0 {
		return nil, false
	}
	end := char
	for end < len(line) {
		ch := line[end]
		if ch == '\\' {
			end += 2
			continue
		}
		if ch == ctx.Quote[0] {
			break
		}
		end++
	}
	if end > len(line) {
		end = len(line)
	}
	name := line[start:end]
	if name == "" {
		return nil, false
	}
	ctx.Name = name
	return ctx, true
}

type viewCall struct {
	argsStart int
	targetArg int
}

func findViewCall(prefix string) (viewCall, bool) {
	for idx := strings.LastIndex(prefix, "view("); idx >= 0; idx = strings.LastIndex(prefix[:idx], "view(") {
		if idx > 0 && isWordChar(prefix[idx-1]) {
			continue
		}
		call := viewCall{argsStart: idx + len("view(")}
		before := strings.TrimSpace(prefix[:idx])
		if strings.HasSuffix(before, "::") || strings.HasSuffix(before, "->") {
			call.targetArg = 1
		}
		return call, true
	}
	return viewCall{}, false
}

func isWordChar(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}
