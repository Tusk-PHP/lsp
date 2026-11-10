package completion

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Alias styles for an import whose short name is already in use in the file
// (the `autoImport.aliasStyle` setting).
const (
	// AliasStyleSuffix imports App\Models\User as UserModel.
	AliasStyleSuffix = "suffix"
	// AliasStylePrefix imports App\Models\User as ModelUser.
	AliasStylePrefix = "prefix"
	// AliasStyleFullyQualified adds no import and inserts \App\Models\User.
	AliasStyleFullyQualified = "fullyQualified"
)

// normalizeAliasStyle returns a known alias style, defaulting to suffix.
func normalizeAliasStyle(style string) string {
	switch style {
	case AliasStylePrefix, AliasStyleFullyQualified:
		return style
	}
	return AliasStyleSuffix
}

// Kinds of name an import brings in. PHP keeps them apart: `use function`
// and `use const` never clash with a class name.
type importKind int

const (
	importClass importKind = iota
	importFunction
	importConst
	importKinds
)

// importKindOf maps a `use` statement's kind to an importKind.
func importKindOf(kind string) importKind {
	switch kind {
	case "function":
		return importFunction
	case "const":
		return importConst
	}
	return importClass
}

// importPlanner decides, for one completion request, how to refer to a
// symbol from another namespace: by its short name with a `use` added, by
// an alias when the short name is taken, or fully qualified. It is built
// once per request so the file's names and the insertion line are worked
// out once, not per item.
type importPlanner struct {
	source string
	file   *parser.FileNode
	index  *symbols.Index
	style  string

	// Per kind: lower-cased name brought in by a `use` → the FQN.
	imported [importKinds]map[string]string
	// Per kind: lower-cased FQN imported → the name it was imported as.
	importedAs [importKinds]map[string]string
	// Per kind: lower-cased last segments of the FQNs imported, so most
	// items skip lower-casing their whole FQN.
	importedShort [importKinds]map[string]bool
	// Per kind: lower-cased names declared in the file.
	declared [importKinds]map[string]bool
	// Per kind: lower-cased short names of the symbols in the file's
	// namespace → their FQNs, filled from the index on first use.
	sameNamespace       [importKinds]map[string]string
	sameNamespaceLoaded bool

	insertLine int // -1 until computed
}

func (p *Provider) newImportPlanner(source string, file *parser.FileNode) *importPlanner {
	pl := &importPlanner{
		source:     source,
		file:       file,
		index:      p.index,
		style:      normalizeAliasStyle(p.aliasStyle),
		insertLine: -1,
	}
	if file == nil {
		return pl
	}
	for _, u := range file.Uses {
		kind := importKindOf(u.Kind)
		fqn := strings.TrimPrefix(u.FullName, "\\")
		lower := strings.ToLower(fqn)
		name := u.Alias
		if name == "" {
			name = shortName(fqn)
		}
		if pl.imported[kind] == nil {
			pl.imported[kind] = map[string]string{}
			pl.importedAs[kind] = map[string]string{}
			pl.importedShort[kind] = map[string]bool{}
		}
		pl.imported[kind][strings.ToLower(name)] = fqn
		if _, ok := pl.importedAs[kind][lower]; !ok {
			pl.importedAs[kind][lower] = name
		}
		pl.importedShort[kind][shortName(lower)] = true
	}
	declare := func(kind importKind, name string) {
		if name == "" {
			return
		}
		if pl.declared[kind] == nil {
			pl.declared[kind] = map[string]bool{}
		}
		pl.declared[kind][strings.ToLower(name)] = true
	}
	for _, c := range file.Classes {
		declare(importClass, c.Name)
	}
	for _, c := range file.Interfaces {
		declare(importClass, c.Name)
	}
	for _, c := range file.Traits {
		declare(importClass, c.Name)
	}
	for _, c := range file.Enums {
		declare(importClass, c.Name)
	}
	for _, f := range file.Functions {
		declare(importFunction, f.Name)
	}
	for _, c := range file.Constants {
		declare(importConst, c.Name)
	}
	return pl
}

// class returns the text to insert for the class-like fqn and the edits that
// make it resolve: a `use` statement, an aliased one, or none.
func (pl *importPlanner) class(fqn string) (string, []protocol.TextEdit) {
	return pl.plan(importClass, fqn)
}

func (pl *importPlanner) plan(kind importKind, fqn string) (string, []protocol.TextEdit) {
	fqn = strings.TrimPrefix(fqn, "\\")
	short := shortName(fqn)
	if fqn == "" || pl.file == nil {
		return short, nil
	}
	// No import needed for unnamespaced symbols.
	if !strings.Contains(fqn, "\\") {
		return short, nil
	}
	var buf [64]byte
	// Already imported, maybe under an alias: use the name it has here.
	if pl.importedShort[kind][string(foldKey(&buf, short))] {
		if name, ok := pl.importedAs[kind][strings.ToLower(fqn)]; ok {
			return name, nil
		}
	}
	// In the file's own namespace, the short name already resolves to it
	// unless an import took that name.
	if pl.file.Namespace != "" && strings.EqualFold(namespaceOf(fqn), pl.file.Namespace) {
		if _, taken := pl.imported[kind][string(foldKey(&buf, short))]; !taken {
			return short, nil
		}
	} else if !pl.taken(kind, short, fqn) {
		return short, pl.useEdit(kind, fqn, "")
	}

	if pl.style == AliasStyleFullyQualified {
		return "\\" + fqn, nil
	}
	if alias := pl.alias(kind, fqn); alias != "" {
		return alias, pl.useEdit(kind, fqn, alias)
	}
	return "\\" + fqn, nil
}

// taken reports whether name already refers to something else in the file:
// an import, a declaration in the file, or a symbol in the file's namespace.
// PHP compares these names without regard to case.
func (pl *importPlanner) taken(kind importKind, name, fqn string) bool {
	var buf [64]byte
	key := foldKey(&buf, name)
	if other, ok := pl.imported[kind][string(key)]; ok && !strings.EqualFold(other, fqn) {
		return true
	}
	if pl.declared[kind][string(key)] {
		return true
	}
	other, ok := pl.namespaceMembers(kind)[string(key)]
	return ok && !strings.EqualFold(other, fqn)
}

// namespaceMembers maps the lower-cased short names of the file's
// namespace's symbols of one kind to their FQNs. It is built
// once per request from the index's namespace map, so each item costs one
// map lookup.
func (pl *importPlanner) namespaceMembers(kind importKind) map[string]string {
	if !pl.sameNamespaceLoaded {
		pl.sameNamespaceLoaded = true
		if pl.file.Namespace != "" && pl.index != nil {
			for _, s := range pl.index.GetNamespaceMembers(pl.file.Namespace) {
				if s == nil || s.ParentFQN != "" {
					continue
				}
				k := importClass
				switch s.Kind {
				case symbols.KindFunction:
					k = importFunction
				case symbols.KindConstant:
					k = importConst
				default:
					if !isClassLike(s) {
						continue
					}
				}
				if pl.sameNamespace[k] == nil {
					pl.sameNamespace[k] = map[string]string{}
				}
				pl.sameNamespace[k][strings.ToLower(s.Name)] = s.FQN
			}
		}
	}
	return pl.sameNamespace[kind]
}

// alias builds a free alias from the namespace segments above the short
// name, singularized: App\Models\User → UserModel (suffix) or ModelUser
// (prefix). When that is taken it takes in the next segment up as well
// (UserFoundationAuth), and returns "" once every segment is used.
func (pl *importPlanner) alias(kind importKind, fqn string) string {
	segments := strings.Split(fqn, "\\")
	short := segments[len(segments)-1]
	parents := segments[:len(segments)-1]
	var b strings.Builder
	for n := 1; n <= len(parents); n++ {
		b.Reset()
		if pl.style == AliasStylePrefix {
			for _, seg := range parents[len(parents)-n:] {
				b.WriteString(singularize(seg))
			}
			b.WriteString(short)
		} else {
			b.WriteString(short)
			for _, seg := range parents[len(parents)-n:] {
				b.WriteString(singularize(seg))
			}
		}
		if alias := b.String(); !pl.taken(kind, alias, fqn) {
			return alias
		}
	}
	return ""
}

// useEdit returns the edit adding `use [function|const] fqn [as alias];`.
func (pl *importPlanner) useEdit(kind importKind, fqn, alias string) []protocol.TextEdit {
	var b strings.Builder
	b.Grow(len("use const ") + len(fqn) + len(" as ") + len(alias) + len(";\n"))
	b.WriteString("use ")
	switch kind {
	case importFunction:
		b.WriteString("function ")
	case importConst:
		b.WriteString("const ")
	}
	b.WriteString(fqn)
	if alias != "" {
		b.WriteString(" as ")
		b.WriteString(alias)
	}
	b.WriteString(";\n")
	line := pl.useInsertLine()
	return []protocol.TextEdit{{
		Range: protocol.Range{
			Start: protocol.Position{Line: line, Character: 0},
			End:   protocol.Position{Line: line, Character: 0},
		},
		NewText: b.String(),
	}}
}

// useInsertLine is the line a new `use` goes on: after the last one, else
// after the namespace declaration with a blank line, else after `<?php`.
func (pl *importPlanner) useInsertLine() int {
	if pl.insertLine >= 0 {
		return pl.insertLine
	}
	line := 0
	if len(pl.file.Uses) > 0 {
		line = pl.file.Uses[len(pl.file.Uses)-1].StartLine + 1
	} else {
		rest := pl.source
		for i := 0; rest != ""; i++ {
			cur := rest
			if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
				cur, rest = rest[:nl], rest[nl+1:]
			} else {
				rest = ""
			}
			if strings.HasPrefix(strings.TrimSpace(cur), "namespace ") {
				line = i + 2 // blank line after namespace
				break
			}
		}
		if line == 0 {
			line = 1 // no namespace: after <?php
		}
	}
	pl.insertLine = line
	return line
}

func isClassLike(s *symbols.Symbol) bool {
	switch s.Kind {
	case symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
		return true
	}
	return false
}

// foldKey lower-cases an ASCII s into buf, so that a lookup such as
// m[string(foldKey(&buf, s))] doesn't allocate. Longer or non-ASCII names
// fall back to strings.ToLower.
func foldKey(buf *[64]byte, s string) []byte {
	if len(s) > len(buf) {
		return []byte(strings.ToLower(s))
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			return []byte(strings.ToLower(s))
		}
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		buf[i] = c
	}
	return buf[:len(s)]
}

func shortName(fqn string) string {
	if i := strings.LastIndexByte(fqn, '\\'); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

func namespaceOf(fqn string) string {
	if i := strings.LastIndexByte(fqn, '\\'); i >= 0 {
		return fqn[:i]
	}
	return ""
}

// singularize turns a plural namespace segment into its singular with a few
// English rules, leaving anything that doesn't look plural alone:
// Categories → Category, Statuses → Status, Boxes → Box, Models → Model,
// Http → Http, Status → Status. A word like Causes comes out as Caus; the
// alias is still a valid, unique name.
func singularize(word string) string {
	n := len(word)
	if n <= 3 {
		return word
	}
	lower := strings.ToLower(word)
	switch {
	case strings.HasSuffix(lower, "ies"):
		return word[:n-3] + matchCase(word[n-3:n-2], "y")
	case strings.HasSuffix(lower, "es"):
		stem := lower[:n-2]
		for _, end := range []string{"ss", "us", "x", "ch", "sh"} {
			if strings.HasSuffix(stem, end) {
				return word[:n-2]
			}
		}
	}
	if strings.HasSuffix(lower, "s") {
		for _, end := range []string{"ss", "us", "is"} {
			if strings.HasSuffix(lower, end) {
				return word
			}
		}
		return word[:n-1]
	}
	return word
}

// matchCase returns s upper-cased when ref is upper-case.
func matchCase(ref, s string) string {
	if ref != "" && ref == strings.ToUpper(ref) && ref != strings.ToLower(ref) {
		return strings.ToUpper(s)
	}
	return s
}
