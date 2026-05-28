// Package manifest provides a position-aware parser for composer.json
// documents, exposing only the structure needed for hover/definition on
// require / require-dev entries and for the repositories override map.
//
// The scanner is hand-rolled (rather than using encoding/json) because
// stdlib JSON does not expose byte offsets for string tokens, and
// because composer.json files under edit can be partially malformed.
// The parser is fault-tolerant — bail-out on unrecoverable syntax, still
// returning whatever entries it managed to recognize before the failure.
package manifest

import (
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// RequireKind identifies which composer require-style block an entry came
// from.
type RequireKind int

const (
	RequireKindNone RequireKind = iota
	// RequireKindRuntime is the top-level "require" block.
	RequireKindRuntime
	// RequireKindDev is the top-level "require-dev" block.
	RequireKindDev
)

// RequireEntry is one package entry inside require or require-dev.
//
// NameRange and ValueRange both refer to the *string content* — they do not
// include the surrounding double quotes. This matches the user's intuition
// when clicking on the name or constraint.
type RequireEntry struct {
	Kind       RequireKind
	Name       string
	NameRange  protocol.Range
	Value      string
	ValueRange protocol.Range
}

// RepositoryEntry is one entry from the top-level "repositories" array.
// Only the type and URL are tracked — that's what the URL resolver needs.
type RepositoryEntry struct {
	Type string
	URL  string
}

// Manifest is the parsed view of a composer.json document.
type Manifest struct {
	Requires     []RequireEntry
	Repositories []RepositoryEntry
}

// Parse scans the given composer.json source and returns its Manifest.
// Returns a non-nil Manifest even on malformed input — fields will simply
// be partial.
func Parse(source string) *Manifest {
	m := &Manifest{}
	s := &scanner{src: source}
	s.skipWS()
	if s.peek() != '{' {
		return m
	}
	s.advance()
	m.parseObject(s, "")
	return m
}

// PackageAtCursor returns the require entry whose name or value range
// contains the cursor, or nil if the cursor is somewhere else.
func (m *Manifest) PackageAtCursor(pos protocol.Position) *RequireEntry {
	for i := range m.Requires {
		r := &m.Requires[i]
		if containsPos(r.NameRange, pos) || containsPos(r.ValueRange, pos) {
			return r
		}
	}
	return nil
}

// containsPos reports whether r covers pos. r.End is exclusive in column,
// matching the convention used elsewhere in the codebase.
func containsPos(r protocol.Range, pos protocol.Position) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}
	if pos.Line == r.Start.Line && pos.Character < r.Start.Character {
		return false
	}
	if pos.Line == r.End.Line && pos.Character > r.End.Character {
		return false
	}
	return true
}

// parseObject parses an object body, assuming the opening { was already
// consumed. The current key path is path (e.g. "" for the root, "require"
// when inside the require block).
func (m *Manifest) parseObject(s *scanner, path string) {
	for {
		s.skipWS()
		c := s.peek()
		if c == 0 || c == '}' {
			if c == '}' {
				s.advance()
			}
			return
		}
		if c == ',' {
			s.advance()
			continue
		}
		if c != '"' {
			// Unexpected token — try to recover by skipping a value.
			if !s.skipValue() {
				return
			}
			continue
		}
		keyValue, keyRange, ok := s.readString()
		if !ok {
			return
		}
		s.skipWS()
		if s.peek() != ':' {
			// Malformed — try to recover.
			s.skipValue()
			continue
		}
		s.advance() // ':'
		s.skipWS()

		switch path {
		case "":
			m.handleRootKey(s, keyValue)
		case "require", "require-dev":
			m.handleRequireValue(s, path, keyValue, keyRange)
		default:
			s.skipValue()
		}
	}
}

func (m *Manifest) handleRootKey(s *scanner, key string) {
	switch key {
	case "require", "require-dev":
		if s.peek() != '{' {
			s.skipValue()
			return
		}
		s.advance()
		m.parseObject(s, key)
	case "repositories":
		m.parseRepositories(s)
	default:
		s.skipValue()
	}
}

func (m *Manifest) handleRequireValue(s *scanner, path, name string, nameRange protocol.Range) {
	if s.peek() != '"' {
		// Object-style constraint (rare in composer.json — e.g. branch alias).
		// We still record the entry, with empty value.
		m.recordRequire(path, name, nameRange, "", protocol.Range{})
		s.skipValue()
		return
	}
	val, valRange, ok := s.readString()
	if !ok {
		m.recordRequire(path, name, nameRange, "", protocol.Range{})
		return
	}
	m.recordRequire(path, name, nameRange, val, valRange)
}

func (m *Manifest) recordRequire(path, name string, nameRange protocol.Range, value string, valueRange protocol.Range) {
	kind := RequireKindRuntime
	if path == "require-dev" {
		kind = RequireKindDev
	}
	m.Requires = append(m.Requires, RequireEntry{
		Kind:       kind,
		Name:       name,
		NameRange:  nameRange,
		Value:      value,
		ValueRange: valueRange,
	})
}

func (m *Manifest) parseRepositories(s *scanner) {
	// repositories can be an array or an object. Most projects use an array.
	switch s.peek() {
	case '[':
		s.advance()
		for {
			s.skipWS()
			c := s.peek()
			if c == 0 || c == ']' {
				if c == ']' {
					s.advance()
				}
				return
			}
			if c == ',' {
				s.advance()
				continue
			}
			if c == '{' {
				s.advance()
				m.parseRepositoryEntry(s)
			} else {
				if !s.skipValue() {
					return
				}
			}
		}
	case '{':
		s.advance()
		// Object form: each value is a repository entry keyed by name.
		for {
			s.skipWS()
			c := s.peek()
			if c == 0 || c == '}' {
				if c == '}' {
					s.advance()
				}
				return
			}
			if c == ',' {
				s.advance()
				continue
			}
			if c != '"' {
				s.skipValue()
				continue
			}
			if _, _, ok := s.readString(); !ok {
				return
			}
			s.skipWS()
			if s.peek() != ':' {
				s.skipValue()
				continue
			}
			s.advance()
			s.skipWS()
			if s.peek() == '{' {
				s.advance()
				m.parseRepositoryEntry(s)
			} else {
				s.skipValue()
			}
		}
	default:
		s.skipValue()
	}
}

// parseRepositoryEntry parses the body of one repositories[] object,
// assuming the opening { was already consumed.
func (m *Manifest) parseRepositoryEntry(s *scanner) {
	entry := RepositoryEntry{}
	for {
		s.skipWS()
		c := s.peek()
		if c == 0 || c == '}' {
			if c == '}' {
				s.advance()
			}
			break
		}
		if c == ',' {
			s.advance()
			continue
		}
		if c != '"' {
			s.skipValue()
			continue
		}
		key, _, ok := s.readString()
		if !ok {
			break
		}
		s.skipWS()
		if s.peek() != ':' {
			s.skipValue()
			continue
		}
		s.advance()
		s.skipWS()
		switch key {
		case "type":
			if s.peek() == '"' {
				val, _, _ := s.readString()
				entry.Type = val
			} else {
				s.skipValue()
			}
		case "url":
			if s.peek() == '"' {
				val, _, _ := s.readString()
				entry.URL = val
			} else {
				s.skipValue()
			}
		default:
			s.skipValue()
		}
	}
	if entry.Type != "" || entry.URL != "" {
		m.Repositories = append(m.Repositories, entry)
	}
}
