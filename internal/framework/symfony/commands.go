package symfony

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// CommandEntry describes a discovered Symfony console command.
type CommandEntry struct {
	Name        string
	Description string
	ClassFQN    string
	URI         string
	Range       textpos.Range
}

// CommandIndex holds all discovered Symfony commands for the project. It is
// safe for concurrent use: the server updates it as files change while
// requests read it.
type CommandIndex struct {
	grammars phpparser.GrammarRange
	mu       sync.RWMutex
	byURI    map[string][]CommandEntry
	entries  []CommandEntry // by name, one per name; rebuilt on change
}

// NewCommandIndex returns an empty CommandIndex that reads files with the
// project's PHP grammars.
func NewCommandIndex(grammars phpparser.GrammarRange) *CommandIndex {
	return &CommandIndex{grammars: grammars, byURI: map[string][]CommandEntry{}}
}

// rebuildLocked recomputes the sorted entries from byURI. A name declared
// twice keeps the entry from the file that sorts first.
func (ci *CommandIndex) rebuildLocked() {
	uris := make([]string, 0, len(ci.byURI))
	for uri := range ci.byURI {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	seen := map[string]bool{}
	entries := make([]CommandEntry, 0, len(ci.entries))
	for _, uri := range uris {
		for _, e := range ci.byURI[uri] {
			if seen[e.Name] {
				continue
			}
			seen[e.Name] = true
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	ci.entries = entries
}

// UpdateFile replaces the commands declared in one project file, after the
// file changed. Its cost grows with the file only.
func (ci *CommandIndex) UpdateFile(uri, source string) {
	if ci == nil || strings.Contains(uri, "/vendor/") {
		return
	}
	var found []CommandEntry
	if mayDeclareCommand(source) {
		found = discoverCommandsInFile(uri, source, ci.grammars)
	}
	ci.mu.Lock()
	defer ci.mu.Unlock()
	if len(found) == 0 && len(ci.byURI[uri]) == 0 {
		return
	}
	if len(found) == 0 {
		delete(ci.byURI, uri)
	} else {
		ci.byURI[uri] = found
	}
	ci.rebuildLocked()
}

// ReplaceWith makes ci hold the commands other holds.
func (ci *CommandIndex) ReplaceWith(other *CommandIndex) {
	if ci == nil || other == nil {
		return
	}
	other.mu.RLock()
	byURI := make(map[string][]CommandEntry, len(other.byURI))
	for uri, entries := range other.byURI {
		byURI[uri] = entries
	}
	other.mu.RUnlock()
	ci.mu.Lock()
	ci.byURI = byURI
	ci.rebuildLocked()
	ci.mu.Unlock()
}

// All returns all discovered command entries sorted by name.
func (ci *CommandIndex) All() []CommandEntry {
	if ci == nil {
		return nil
	}
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return append([]CommandEntry(nil), ci.entries...)
}

// Find returns the entry for the given command name or nil.
func (ci *CommandIndex) Find(name string) *CommandEntry {
	if ci == nil {
		return nil
	}
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	for i := range ci.entries {
		if ci.entries[i].Name == name {
			e := ci.entries[i]
			return &e
		}
	}
	return nil
}

// Names returns all command names in sorted order.
func (ci *CommandIndex) Names() []string {
	if ci == nil {
		return nil
	}
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	names := make([]string, 0, len(ci.entries))
	for _, e := range ci.entries {
		names = append(names, e.Name)
	}
	return names
}

// mayDeclareCommand is a quick check before a file is read for commands.
func mayDeclareCommand(source string) bool {
	return strings.Contains(source, "AsCommand") && strings.Contains(source, "#[") || strings.Contains(source, "$defaultName")
}

const asCommandFQN = `Symfony\Component\Console\Attribute\AsCommand`

// DiscoverCommands scans indexed project PHP sources for Symfony console commands,
// recognising both #[AsCommand(...)] attributes and legacy $defaultName properties.
func DiscoverCommands(index *symbols.Index) *CommandIndex {
	ci := NewCommandIndex(index.Grammars())

	for _, uri := range index.GetAllFileURIs() {
		if strings.Contains(uri, "/vendor/") {
			continue
		}
		source := index.GetFileSource(uri)
		if source == "" || !mayDeclareCommand(source) {
			continue
		}
		if found := discoverCommandsInFile(uri, source, index.GrammarOf(uri)); len(found) > 0 {
			ci.byURI[uri] = found
		}
	}

	// Also scan project src/ directories directly for files not yet indexed
	for _, root := range discoverSymfonyRoots(index) {
		discoverCommandsFromDir(filepath.Join(root, "src"), ci.byURI, ci.grammars)
	}

	ci.rebuildLocked()
	return ci
}

func discoverCommandsFromDir(dir string, byURI map[string][]CommandEntry, grammars phpparser.GrammarRange) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		src := string(data)
		if !mayDeclareCommand(src) {
			return nil
		}
		abs := path
		if a, err := filepath.Abs(path); err == nil {
			abs = a
		}
		uri := "file://" + filepath.ToSlash(abs)
		if _, ok := byURI[uri]; ok {
			return nil
		}
		if found := discoverCommandsInFile(uri, src, grammars); len(found) > 0 {
			byURI[uri] = found
		}
		return nil
	})
}

func discoverCommandsInFile(uri, source string, grammars phpparser.GrammarRange) []CommandEntry {
	file := parser.ParseFileIn(source, grammars)
	var entries []CommandEntry
	add := func(cls parser.ClassNode, name, description string, line, col, width int) {
		entries = append(entries, CommandEntry{
			Name:        name,
			Description: description,
			ClassFQN:    cls.FullName,
			URI:         uri,
			Range: textpos.Range{
				Start: textpos.Position{Line: line, Character: col},
				End:   textpos.Position{Line: line, Character: col + width},
			},
		})
	}
	for _, cls := range file.Classes {
		if name, description, ok := asCommand(file, cls); ok {
			add(cls, name, description, cls.StartLine, cls.StartCol, len(cls.Name))
		}
		if prop, ok := staticStringProperty(cls, "$defaultName"); ok {
			name, _ := parser.StringLiteral(prop.DefaultValue)
			description := ""
			if desc, ok := staticStringProperty(cls, "$defaultDescription"); ok {
				description, _ = parser.StringLiteral(desc.DefaultValue)
			}
			add(cls, name, description, prop.StartLine, prop.StartCol, len(prop.Name))
		}
	}
	return entries
}

func asCommand(file *parser.FileNode, cls parser.ClassNode) (name, description string, ok bool) {
	for _, attr := range cls.Attributes {
		if !strings.EqualFold(file.ResolveClassName(attr.Name), asCommandFQN) {
			continue
		}
		value, _ := attr.Args.Get("name", 0)
		name, ok = parser.StringLiteral(value)
		if !ok || name == "" {
			return "", "", false
		}
		value, _ = attr.Args.Get("description", 1)
		description, _ = parser.StringLiteral(value)
		return name, description, true
	}
	return "", "", false
}

func staticStringProperty(cls parser.ClassNode, name string) (parser.PropertyNode, bool) {
	for _, prop := range cls.Properties {
		if prop.Name == name && prop.IsStatic {
			if value, ok := parser.StringLiteral(prop.DefaultValue); ok && (value != "" || name != "$defaultName") {
				return prop, true
			}
		}
	}
	return parser.PropertyNode{}, false
}

// ExtractCommandNameContext detects if the cursor is inside a command-name
// string argument to $application->find('...') or similar patterns.
// Returns (partial, quote, ok).
func ExtractCommandNameContext(trimmed string) (partial, quote string, ok bool) {
	patterns := []string{
		"->find(",
		"->get(",
		"->run(",
	}
	for _, pat := range patterns {
		idx := strings.LastIndex(trimmed, pat)
		if idx < 0 {
			continue
		}
		after := trimmed[idx+len(pat):]
		if strings.Contains(after, ")") || strings.Contains(after, ",") {
			continue
		}
		after = strings.TrimLeft(after, " \t")
		if len(after) > 0 && (after[0] == '\'' || after[0] == '"') {
			return after[1:], string(after[0]), true
		}
		return after, "", true
	}
	return "", "", false
}

// consoleApplicationFQN is the class whose find(), get() and has() take a
// console command name.
const consoleApplicationFQN = "Symfony\\Component\\Console\\Application"

// commandNameMethods are the console Application methods whose first
// argument is a command name.
var commandNameMethods = []string{"find", "get", "has"}

// IsConsoleApplication reports whether fqn is Symfony's console Application
// or a class extending it, such as FrameworkBundle's.
func IsConsoleApplication(index *symbols.Index, fqn string) bool {
	fqn = strings.TrimPrefix(fqn, "\\")
	if fqn == consoleApplicationFQN {
		return true
	}
	if index == nil || fqn == "" {
		return false
	}
	for _, parent := range index.GetInheritanceChain(fqn) {
		if strings.TrimPrefix(parent, "\\") == consoleApplicationFQN {
			return true
		}
	}
	return false
}
