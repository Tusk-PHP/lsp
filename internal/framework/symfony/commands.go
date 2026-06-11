package symfony

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// CommandEntry describes a discovered Symfony console command.
type CommandEntry struct {
	Name        string
	Description string
	ClassFQN    string
	URI         string
	Range       protocol.Range
}

// CommandIndex holds all discovered Symfony commands for the project.
type CommandIndex struct {
	entries []CommandEntry
}

// All returns all discovered command entries sorted by name.
func (ci *CommandIndex) All() []CommandEntry {
	if ci == nil {
		return nil
	}
	return ci.entries
}

// Find returns the entry for the given command name or nil.
func (ci *CommandIndex) Find(name string) *CommandEntry {
	if ci == nil {
		return nil
	}
	for i := range ci.entries {
		if ci.entries[i].Name == name {
			return &ci.entries[i]
		}
	}
	return nil
}

// Names returns all command names in sorted order.
func (ci *CommandIndex) Names() []string {
	if ci == nil {
		return nil
	}
	names := make([]string, 0, len(ci.entries))
	for _, e := range ci.entries {
		names = append(names, e.Name)
	}
	return names
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
	if index == nil {
		return &CommandIndex{}
	}

	var entries []CommandEntry
	seen := make(map[string]bool)

	for _, uri := range index.GetAllFileURIs() {
		if strings.Contains(uri, "/vendor/") {
			continue
		}
		source := index.GetFileSource(uri)
		if source == "" {
			continue
		}
		// Quick scan: skip files without relevant content
		hasAttribute := strings.Contains(source, "AsCommand") && strings.Contains(source, "#[")
		hasLegacy := strings.Contains(source, "$defaultName")
		if !hasAttribute && !hasLegacy {
			continue
		}

		for _, entry := range discoverCommandsInFile(uri, source) {
			if seen[entry.Name] {
				continue
			}
			seen[entry.Name] = true
			entries = append(entries, entry)
		}
	}

	// Also scan project src/ directories directly for files not yet indexed
	for _, root := range discoverSymfonyRoots(index) {
		srcDir := filepath.Join(root, "src")
		discoverCommandsFromDir(srcDir, seen, &entries)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return &CommandIndex{entries: entries}
}

func discoverCommandsFromDir(dir string, seen map[string]bool, entries *[]CommandEntry) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		src := string(data)
		hasAttribute := strings.Contains(src, "AsCommand") && strings.Contains(src, "#[")
		hasLegacy := strings.Contains(src, "$defaultName")
		if !hasAttribute && !hasLegacy {
			return nil
		}
		abs := path
		if a, err := filepath.Abs(path); err == nil {
			abs = a
		}
		uri := "file://" + filepath.ToSlash(abs)
		for _, entry := range discoverCommandsInFile(uri, src) {
			if seen[entry.Name] {
				continue
			}
			seen[entry.Name] = true
			*entries = append(*entries, entry)
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
				rng := protocol.Range{
					Start: protocol.Position{Line: i, Character: leadingYAMLIndent(line)},
					End:   protocol.Position{Line: i, Character: leadingYAMLIndent(line) + len(trimmed)},
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
				rng := protocol.Range{
					Start: protocol.Position{Line: i, Character: leadingYAMLIndent(line)},
					End:   protocol.Position{Line: i, Character: leadingYAMLIndent(line) + len(trimmed)},
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
