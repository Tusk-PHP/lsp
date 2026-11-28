package symbols

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// qualifyDocTypes rewrites the class names a member's PHPDoc types take from
// the file's `use` imports to fully qualified names, in its @param, @return,
// @var and @throws tags, the generic tags (@template bounds and defaults,
// @extends, @implements, @use, @mixin, @param-out, @phpstan-assert and the
// like) and class-level @method and @property tags, in their plain,
// @phpstan- and @psalm- forms.
//
// A PHPDoc type means what the imports of the file that declares it say, but
// it is read where the member is used, where another class may have the same
// short name. Qualifying it once, while the declaring file's imports are at
// hand, keeps `@return Product` pointing at the declaring file's Product.
//
// Only imported names are rewritten. Other unqualified names may be template
// parameters or type aliases declared elsewhere, so they stay as written. The
// doc comment is returned unchanged, without allocating, when nothing in it is
// imported.
func qualifyDocTypes(doc string, uses []parser.UseNode) string {
	if doc == "" || len(uses) == 0 {
		return doc
	}
	var b strings.Builder
	last := 0
	forEachDocType(doc, func(ts int, typeStr string) {
		forEachImportedName(typeStr, uses, func(start, end int, fqn string) {
			if b.Len() == 0 {
				b.Grow(len(doc) + 64)
			}
			b.WriteString(doc[last : ts+start])
			b.WriteByte('\\')
			b.WriteString(fqn)
			last = ts + end
		})
	})
	if last == 0 {
		return doc
	}
	b.WriteString(doc[last:])
	return b.String()
}

// ForEachDocClassName calls fn with each class name the type tags of a PHPDoc
// comment use, as written (a fully qualified one with its leading backslash),
// and its byte offsets in doc. Variables, literals and shape keys are not
// names; template parameters and type keywords are, since only the file's
// imports can tell them apart.
func ForEachDocClassName(doc string, fn func(start, end int, name string)) {
	forEachDocType(doc, func(ts int, typeStr string) {
		forEachClassName(typeStr, func(start, end int) {
			fn(ts+start, ts+end, typeStr[start:end])
		})
	})
}

// forEachDocType calls fn with each type a PHPDoc comment's tags give and
// where it starts in doc.
func forEachDocType(doc string, fn func(ts int, typeStr string)) {
	i := 0
	for {
		k := strings.IndexByte(doc[i:], '@')
		if k < 0 {
			return
		}
		at := i + k
		j := at + 1
		for j < len(doc) && (isDocNameByte(doc[j]) || doc[j] == '-') {
			j++
		}
		i = j
		tag := doc[at+1 : j]
		tag = strings.TrimPrefix(strings.TrimPrefix(tag, "phpstan-"), "psalm-")
		lineEnd := strings.IndexByte(doc[j:], '\n')
		if lineEnd < 0 {
			lineEnd = len(doc)
		} else {
			lineEnd += j
		}
		starts, n := docTagTypeStarts(tag, doc, j, lineEnd)
		for _, ts := range starts[:n] {
			if ts < i {
				continue
			}
			typeStr, _ := types.ExtractDocTypeString(doc[ts:lineEnd])
			if typeStr == "" || !strings.HasPrefix(doc[ts:], typeStr) {
				continue
			}
			fn(ts, typeStr)
			if ts+len(typeStr) > i {
				i = ts + len(typeStr)
			}
		}
	}
}

// forEachImportedName calls fn for each class name in a PHPDoc type whose first
// segment is a `use` import, with its position and fully qualified name.
func forEachImportedName(t string, uses []parser.UseNode, fn func(start, end int, fqn string)) {
	forEachClassName(t, func(start, end int) {
		if t[start] == '\\' {
			return
		}
		if fqn, ok := importedName(t[start:end], uses); ok {
			fn(start, end, fqn)
		}
	})
}

// forEachClassName calls fn with the position of each class name in a PHPDoc
// type, fully qualified ones included. Variables, quoted literals, numbers and
// shape keys are skipped.
func forEachClassName(t string, fn func(start, end int)) {
	braces := 0
	for i := 0; i < len(t); {
		c := t[i]
		switch {
		case c == '\'' || c == '"':
			k := strings.IndexByte(t[i+1:], c)
			if k < 0 {
				return
			}
			i += k + 2
			continue
		case c == '{':
			braces++
		case c == '}':
			braces--
		case c == '$' || (c >= '0' && c <= '9'):
			i++
			for i < len(t) && (isDocNameByte(t[i]) || t[i] == '\\') {
				i++
			}
			continue
		case c == '\\' || isDocNameByte(c):
			start := i
			for i < len(t) && (isDocNameByte(t[i]) || t[i] == '\\') {
				i++
			}
			if braces > 0 {
				k := i
				if k < len(t) && t[k] == '?' {
					k++
				}
				if k < len(t) && t[k] == ':' && (k+1 >= len(t) || t[k+1] != ':') {
					continue
				}
			}
			fn(start, i)
			continue
		}
		i++
	}
}

// importedName returns the fully qualified name of name when its first segment
// is a class import.
func importedName(name string, uses []parser.UseNode) (string, bool) {
	first, rest, nested := strings.Cut(name, "\\")
	for _, u := range uses {
		if u.Alias != first || (u.Kind != "" && u.Kind != "class") {
			continue
		}
		full := strings.TrimPrefix(u.FullName, "\\")
		if nested {
			return full + "\\" + rest, true
		}
		return full, true
	}
	return "", false
}

// docTagTypeStarts returns where the types of a tag's value start in doc. The
// tag's name ends at j and its line at lineEnd. Most tags start with a type;
// @template has one after `of` and another after `=`.
func docTagTypeStarts(tag, doc string, j, lineEnd int) (starts [2]int, n int) {
	ts := j
	for ts < lineEnd && (doc[ts] == ' ' || doc[ts] == '\t') {
		ts++
	}
	if ts >= lineEnd {
		return starts, 0
	}
	switch tag {
	case "param", "return", "var", "throws", "param-out", "param-closure-this",
		"extends", "implements", "use", "template-extends", "template-implements", "template-use",
		"mixin", "self-out", "this-out", "property", "property-read", "property-write":
		return [2]int{ts}, 1
	case "assert", "assert-if-true", "assert-if-false":
		for ts < lineEnd && (doc[ts] == '!' || doc[ts] == '=') {
			ts++
		}
		return [2]int{ts}, 1
	case "method":
		if strings.HasPrefix(doc[ts:lineEnd], "static ") {
			ts += len("static ")
			for ts < lineEnd && doc[ts] == ' ' {
				ts++
			}
		}
		// Only a return type: `@method name()` has none.
		rest := doc[ts:lineEnd]
		if open := strings.IndexByte(rest, '('); open >= 0 && !strings.ContainsAny(strings.TrimSpace(rest[:open]), " \t") {
			return starts, 0
		}
		return [2]int{ts}, 1
	case "template", "template-covariant", "template-contravariant":
		line := doc[ts:lineEnd]
		for _, kw := range []string{" of ", " as "} {
			if k := strings.Index(line, kw); k >= 0 {
				p := ts + k + len(kw)
				for p < lineEnd && doc[p] == ' ' {
					p++
				}
				starts[n] = p
				n++
				break
			}
		}
		if k := strings.Index(line, "="); k >= 0 {
			p := ts + k + 1
			for p < lineEnd && doc[p] == ' ' {
				p++
			}
			starts[n] = p
			n++
		}
		return starts, n
	}
	return starts, 0
}

func isDocNameByte(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c >= 0x80
}
