package symbols

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
)

func indexTypeAliases(owner *Symbol, doc *parser.DocBlock, resolve func(string) string) {
	if owner == nil || doc == nil {
		return
	}

	for _, tag := range []string{"phpstan-type", "psalm-type"} {
		for _, value := range doc.Tags[tag] {
			name, typeExpr := parseTypeAliasTag(value)
			if name == "" || typeExpr == "" {
				continue
			}
			ensureTypeAliasMap(owner)
			owner.TypeAliases[name] = TypeAlias{
				Name: name,
				Type: resolveDocAliasType(typeExpr, resolve),
			}
		}
	}

	for _, tag := range []string{"phpstan-import-type", "psalm-import-type"} {
		for _, value := range doc.Tags[tag] {
			localName, importedName, fromFQN := parseImportedTypeAliasTag(value, resolve)
			if localName == "" || importedName == "" || fromFQN == "" {
				continue
			}
			ensureTypeAliasMap(owner)
			owner.TypeAliases[localName] = TypeAlias{
				Name: localName,
				Import: &ImportedTypeAlias{
					FromFQN:    fromFQN,
					ImportedAs: importedName,
				},
			}
		}
	}
}

func ensureTypeAliasMap(owner *Symbol) {
	if owner.TypeAliases == nil {
		owner.TypeAliases = make(map[string]TypeAlias)
	}
}

func parseTypeAliasTag(value string) (name, typeExpr string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}

	fields := strings.Fields(value)
	if len(fields) < 2 {
		return "", ""
	}

	name = fields[0]
	typeExpr = strings.TrimSpace(value[len(name):])
	return name, typeExpr
}

func parseImportedTypeAliasTag(value string, resolve func(string) string) (localName, importedName, fromFQN string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", ""
	}

	fields := strings.Fields(value)
	if len(fields) < 3 {
		return "", "", ""
	}

	importedName = fields[0]
	localName = importedName

	fromIdx := -1
	for i := 1; i < len(fields); i++ {
		if fields[i] == "from" {
			fromIdx = i
			break
		}
	}
	if fromIdx < 0 || fromIdx+1 >= len(fields) {
		return "", "", ""
	}

	fromFQN = resolve(fields[fromIdx+1])
	if fromIdx+2 < len(fields) && fields[fromIdx+2] == "as" && fromIdx+3 < len(fields) {
		localName = fields[fromIdx+3]
	}

	return localName, importedName, fromFQN
}

func resolveDocAliasType(raw string, resolve func(string) string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var out strings.Builder
	tokenStart := -1
	flush := func(end int) {
		if tokenStart < 0 {
			return
		}
		token := raw[tokenStart:end]
		if isShapeKey(raw, tokenStart, end) {
			out.WriteString(token)
		} else {
			out.WriteString(resolveAliasToken(token, resolve))
		}
		tokenStart = -1
	}
	inString := byte(0)

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString != 0 {
			flush(i)
			out.WriteByte(ch)
			if ch == inString && (i == 0 || raw[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			flush(i)
			inString = ch
			out.WriteByte(ch)
			continue
		}
		if isAliasTokenChar(ch) {
			if tokenStart < 0 {
				tokenStart = i
			}
			continue
		}
		flush(i)
		out.WriteByte(ch)
	}
	flush(len(raw))

	return out.String()
}

func isShapeKey(raw string, start, end int) bool {
	next := end
	for next < len(raw) && (raw[next] == ' ' || raw[next] == '\t') {
		next++
	}
	if next < len(raw) && raw[next] == '?' {
		next++
		for next < len(raw) && (raw[next] == ' ' || raw[next] == '\t') {
			next++
		}
	}
	return next < len(raw) && raw[next] == ':'
}

func resolveAliasToken(token string, resolve func(string) string) string {
	if token == "" {
		return ""
	}
	if IsPHPBuiltinType(token) || token == "self" || token == "static" || token == "$this" {
		return token
	}
	if strings.HasPrefix(token, "$") {
		return token
	}
	return resolve(token)
}

func isAliasTokenChar(ch byte) bool {
	return ch == '\\' || ch == '_' || ch == '$' ||
		(ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9')
}
