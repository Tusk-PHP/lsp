package symfony

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

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
	mu      sync.RWMutex
	byURI   map[string][]CommandEntry
	entries []CommandEntry // by name, one per name; rebuilt on change
}

// NewCommandIndex returns an empty CommandIndex.
func NewCommandIndex() *CommandIndex {
	return &CommandIndex{byURI: map[string][]CommandEntry{}}
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
		found = discoverCommandsInFile(uri, source)
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

// Package-level compiled regexes — anchored and conservative.
var (
	// Matches #[AsCommand(name: 'app:do-thing', description: 'Description here')]
	asCommandNamePattern = regexp.MustCompile(`name\s*:\s*['"]([^'"]+)['"]`)
	// Matches the optional description argument
	asCommandDescPattern = regexp.MustCompile(`description\s*:\s*['"]([^'"]+)['"]`)
	// Matches the first positional string: #[AsCommand('app:do-thing')]
	asCommandFirstStringPattern = regexp.MustCompile(`^\s*['"]([^'"]+)['"]`)
	// Matches legacy protected static $defaultName = 'app:do-thing';
	defaultNamePattern = regexp.MustCompile(`protected\s+static\s+\$defaultName\s*=\s*['"]([^'"]+)['"]`)
	// Matches legacy protected static $defaultDescription = 'Description';
	defaultDescPattern = regexp.MustCompile(`protected\s+static\s+\$defaultDescription\s*=\s*['"]([^'"]+)['"]`)
)

// DiscoverCommands scans indexed project PHP sources for Symfony console commands,
// recognising both #[AsCommand(...)] attributes and legacy $defaultName properties.
func DiscoverCommands(index *symbols.Index) *CommandIndex {
	ci := NewCommandIndex()
	if index == nil {
		return ci
	}

	for _, uri := range index.GetAllFileURIs() {
		if strings.Contains(uri, "/vendor/") {
			continue
		}
		source := index.GetFileSource(uri)
		if source == "" || !mayDeclareCommand(source) {
			continue
		}
		if found := discoverCommandsInFile(uri, source); len(found) > 0 {
			ci.byURI[uri] = found
		}
	}

	// Also scan project src/ directories directly for files not yet indexed
	for _, root := range discoverSymfonyRoots(index) {
		discoverCommandsFromDir(filepath.Join(root, "src"), ci.byURI)
	}

	ci.rebuildLocked()
	return ci
}

func discoverCommandsFromDir(dir string, byURI map[string][]CommandEntry) {
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
		if found := discoverCommandsInFile(uri, src); len(found) > 0 {
			byURI[uri] = found
		}
		return nil
	})
}

func discoverCommandsInFile(uri, source string) []CommandEntry {
	lines := strings.Split(source, "\n")
	var entries []CommandEntry

	// Extract namespace and class FQN from source
	classFQN := extractCommandClassFQN(source)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect #[AsCommand(...)] attribute
		if strings.Contains(trimmed, "#[") && strings.Contains(trimmed, "AsCommand") {
			end := findAttributeEnd(source, indexOf(source, trimmed, i)+2)
			var block string
			if end >= 0 {
				startOff := indexOf(source, trimmed, i)
				if startOff >= 0 && end < len(source) {
					block = source[startOff : end+1]
				}
			}
			if block == "" {
				block = trimmed
			}
			name, desc := parseAsCommandBlock(block)
			if name != "" {
				rng := textpos.Range{
					Start: textpos.Position{Line: i, Character: leadingYAMLIndent(line)},
					End:   textpos.Position{Line: i, Character: leadingYAMLIndent(line) + len(trimmed)},
				}
				entries = append(entries, CommandEntry{
					Name:        name,
					Description: desc,
					ClassFQN:    classFQN,
					URI:         uri,
					Range:       rng,
				})
			}
		}

		// Detect legacy protected static $defaultName = 'app:do-thing';
		if strings.Contains(trimmed, "$defaultName") {
			if m := defaultNamePattern.FindStringSubmatch(trimmed); len(m) >= 2 {
				name := m[1]
				desc := ""
				// Look ahead for $defaultDescription
				for j := i + 1; j < len(lines) && j < i+5; j++ {
					if dm := defaultDescPattern.FindStringSubmatch(strings.TrimSpace(lines[j])); len(dm) >= 2 {
						desc = dm[1]
						break
					}
				}
				rng := textpos.Range{
					Start: textpos.Position{Line: i, Character: leadingYAMLIndent(line)},
					End:   textpos.Position{Line: i, Character: leadingYAMLIndent(line) + len(trimmed)},
				}
				entries = append(entries, CommandEntry{
					Name:        name,
					Description: desc,
					ClassFQN:    classFQN,
					URI:         uri,
					Range:       rng,
				})
			}
		}
	}

	return entries
}

// indexOf finds the byte offset of lineContent in source starting at approximately
// lineNo, used for block extraction.
func indexOf(source, lineContent string, lineNo int) int {
	// Walk to the line
	current := 0
	for i := 0; i < lineNo; i++ {
		nl := strings.Index(source[current:], "\n")
		if nl < 0 {
			return -1
		}
		current += nl + 1
	}
	idx := strings.Index(source[current:], lineContent)
	if idx < 0 {
		return -1
	}
	return current + idx
}

// parseAsCommandBlock extracts name and description from an #[AsCommand(...)] block.
func parseAsCommandBlock(block string) (name, description string) {
	open := strings.Index(block, "(")
	close := strings.LastIndex(block, ")")
	if open < 0 || close <= open {
		return "", ""
	}

	// Verify this is an AsCommand attribute
	namePart := strings.TrimSpace(block[2:open])
	parts := strings.Split(namePart, "\\")
	short := parts[len(parts)-1]
	// Allow both bare "AsCommand" and "Console\Attribute\AsCommand"
	if !strings.HasSuffix(short, "AsCommand") {
		return "", ""
	}

	args := block[open+1 : close]

	// Named form: name: 'app:do-thing'
	if m := asCommandNamePattern.FindStringSubmatch(args); len(m) >= 2 {
		name = m[1]
	}
	// Positional form: first string is the name
	if name == "" {
		if m := asCommandFirstStringPattern.FindStringSubmatch(args); len(m) >= 2 {
			name = m[1]
		}
	}
	// Description
	if m := asCommandDescPattern.FindStringSubmatch(args); len(m) >= 2 {
		description = m[1]
	}
	return name, description
}

// extractCommandClassFQN extracts the FQN of the first class found in the source.
func extractCommandClassFQN(source string) string {
	ns := ""
	lines := strings.Split(source, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "namespace ") {
			ns = strings.TrimSuffix(strings.TrimPrefix(trimmed, "namespace "), ";")
			ns = strings.TrimSpace(ns)
			continue
		}
		if strings.HasPrefix(trimmed, "class ") || strings.Contains(trimmed, " class ") {
			parts := strings.Fields(trimmed)
			for i, p := range parts {
				if p == "class" && i+1 < len(parts) {
					className := parts[i+1]
					// Strip trailing { or other punctuation
					className = strings.TrimRight(className, "{(")
					if ns != "" {
						return ns + "\\" + className
					}
					return className
				}
			}
		}
	}
	return ""
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

// CommandNameCall reports whether prefix, a line up to the cursor, ends in
// the first argument of `->find(`, `->get(` or `->has(`. It returns the
// receiver's expression as written, which the caller must resolve to a
// console application (see IsConsoleApplication) before treating the
// argument as a command name: other classes have get() too. partial is the
// name typed so far and quote the quote it opened with, "" before any.
func CommandNameCall(prefix string) (receiver, partial, quote string, ok bool) {
	open := -1
	for _, m := range commandNameMethods {
		pat := "->" + m + "("
		if i := strings.LastIndex(prefix, pat); i >= 0 && i+len(pat) > open {
			open = i + len(pat)
			receiver = receiverBefore(strings.TrimSuffix(prefix[:i], "?"))
		}
	}
	if open < 0 || receiver == "" {
		return "", "", "", false
	}
	arg := strings.TrimLeft(prefix[open:], " \t")
	if arg == "" {
		return receiver, "", "", true
	}
	if arg[0] != '\'' && arg[0] != '"' {
		return "", "", "", false
	}
	quote, partial = arg[:1], arg[1:]
	if strings.ContainsAny(partial, quote+"\\$") {
		return "", "", "", false // the string closed, or isn't a plain name
	}
	return receiver, partial, quote, true
}

// CommandNameAt returns the command name in the string under cursor on line
// when that string is the first argument of `->find(`, `->get(` or `->has(`,
// with the receiver's expression as written.
func CommandNameAt(line string, cursor int) (receiver, name string, ok bool) {
	if cursor < 0 || cursor > len(line) {
		return "", "", false
	}
	receiver, partial, quote, ok := CommandNameCall(line[:cursor])
	if !ok || quote == "" {
		return "", "", false
	}
	end := strings.Index(line[cursor:], quote)
	if end < 0 {
		return "", "", false
	}
	name = partial + line[cursor:cursor+end]
	if name == "" {
		return "", "", false
	}
	return receiver, name, true
}

// receiverBefore returns the expression that ends at the end of s: a
// variable, property or call chain such as `$this->app` or `$kernel->app()`.
func receiverBefore(s string) string {
	s = strings.TrimRight(s, " \t")
	i := len(s)
	for i > 0 {
		c := s[i-1]
		switch {
		case c == '_' || c == '$' || c == '\\' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9':
			i--
		case c == '>' && i >= 2 && s[i-2] == '-':
			i -= 2
			if i > 0 && s[i-1] == '?' {
				i--
			}
		case c == ':' && i >= 2 && s[i-2] == ':':
			i -= 2
		case c == ')':
			depth := 0
			j := i - 1
			for ; j >= 0; j-- {
				if s[j] == ')' {
					depth++
				} else if s[j] == '(' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			if j < 0 {
				return ""
			}
			i = j
		default:
			return s[i:]
		}
	}
	return s[i:]
}

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
