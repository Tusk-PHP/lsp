package symbols

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// qualifyDocTypes rewrites the class names a member's PHPDoc types take from
// the file's `use` imports to fully qualified names, in every type position of
// every tag the PHPDoc reader knows (parser.ForEachDocTagType), in their
// plain, @phpstan- and @psalm- forms.
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
	i := 0
	for {
		k := strings.IndexByte(doc[i:], '@')
		if k < 0 {
			break
		}
		at := i + k
		j := at + 1
		for j < len(doc) && (isDocNameByte(doc[j]) || doc[j] == '-') {
			j++
		}
		i = j
		lineEnd := strings.IndexByte(doc[j:], '\n')
		if lineEnd < 0 {
			lineEnd = len(doc)
		} else {
			lineEnd += j
		}
		value := doc[j:lineEnd]
		if !namesImport(value, uses) {
			continue
		}
		parser.ForEachDocTagType(doc[at+1:j], value, func(start int, typ string) {
			ts := j + start
			forEachImportedName(typ, uses, func(start, end int, fqn string) {
				if b.Len() == 0 {
					b.Grow(len(doc) + 64)
				}
				b.WriteString(doc[last : ts+start])
				b.WriteByte('\\')
				b.WriteString(fqn)
				last = ts + end
			})
			i = max(i, ts+len(typ))
		})
	}
	if last == 0 {
		return doc
	}
	b.WriteString(doc[last:])
	return b.String()
}

// forEachImportedName calls fn for each class name in a PHPDoc type whose first
// segment is a `use` import, with its position and fully qualified name.
// Variables, quoted literals, shape keys and fully qualified names are skipped.
func forEachImportedName(t string, uses []parser.UseNode, fn func(start, end int, fqn string)) {
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
		case c == '$' || c == '\\' || (c >= '0' && c <= '9'):
			// A variable, a fully qualified name or a number: skip all of it.
			i++
			for i < len(t) && (isDocNameByte(t[i]) || t[i] == '\\') {
				i++
			}
			continue
		case isDocNameByte(c):
			start := i
			for i < len(t) && (isDocNameByte(t[i]) || t[i] == '\\') {
				i++
			}
			// `key: Type` and `key?: Type` in a shape name a key, not a class.
			if braces > 0 {
				k := i
				if k < len(t) && t[k] == '?' {
					k++
				}
				if k < len(t) && t[k] == ':' && (k+1 >= len(t) || t[k+1] != ':') {
					continue
				}
			}
			if fqn, ok := importedName(t[start:i], uses); ok {
				fn(start, i, fqn)
			}
			continue
		}
		i++
	}
}

func namesImport(t string, uses []parser.UseNode) bool {
	found := false
	forEachImportedName(t, uses, func(int, int, string) { found = true })
	return found
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

func isDocNameByte(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c >= 0x80
}
