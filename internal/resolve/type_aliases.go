package resolve

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func (r *Resolver) expandTypeAliases(raw, ownerFQN string, file *parser.FileNode) string {
	return r.expandTypeAliasesSeen(raw, ownerFQN, file, make(map[string]bool))
}

func (r *Resolver) expandTypeAliasesSeen(raw, ownerFQN string, file *parser.FileNode, seen map[string]bool) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || ownerFQN == "" {
		return raw
	}

	var out strings.Builder
	tokenStart := -1
	flush := func(end int) {
		if tokenStart < 0 {
			return
		}
		out.WriteString(r.expandAliasToken(raw[tokenStart:end], ownerFQN, file, seen))
		tokenStart = -1
	}

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
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

func (r *Resolver) expandAliasToken(token, ownerFQN string, file *parser.FileNode, seen map[string]bool) string {
	if token == "" || token == "$this" || token == "self" || token == "static" || symbols.IsPHPBuiltinType(token) {
		return token
	}
	if strings.HasPrefix(token, "\\") || strings.Contains(token, "\\") {
		return strings.TrimPrefix(token, "\\")
	}

	aliasKey := ownerFQN + "::" + token
	if seen[aliasKey] {
		return token
	}

	alias, ok := r.Index.LookupTypeAlias(ownerFQN, token)
	if !ok {
		return token
	}

	seen[aliasKey] = true
	defer delete(seen, aliasKey)

	if alias.Import != nil {
		if imported, ok := r.Index.LookupTypeAlias(alias.Import.FromFQN, alias.Import.ImportedAs); ok {
			if imported.Import != nil {
				return r.expandAliasToken(imported.Name, alias.Import.FromFQN, file, seen)
			}
			return r.expandTypeAliasesSeen(imported.Type, alias.Import.FromFQN, file, seen)
		}
		return token
	}

	return r.expandTypeAliasesSeen(alias.Type, ownerFQN, file, seen)
}

func ownerScopeFQN(member *symbols.Symbol, pos protocol.Position, file *parser.FileNode) string {
	if member != nil {
		if member.ParentFQN != "" {
			return member.ParentFQN
		}
		switch member.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
			return member.FQN
		}
	}
	return FindEnclosingClass(file, pos)
}

func isAliasTokenChar(ch byte) bool {
	return ch == '\\' || ch == '_' || ch == '$' ||
		(ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9')
}
