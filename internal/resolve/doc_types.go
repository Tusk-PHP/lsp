package resolve

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
	internaltypes "github.com/open-southeners/tusk-php/internal/types"
)

func (r *Resolver) resolveDocTypeWithAliases(raw string, parsed *internaltypes.Type, file *parser.FileNode, aliases map[string]*internaltypes.Type) ResolvedType {
	if parsed == nil {
		parsed = internaltypes.Parse(raw)
	}
	if parsed == nil {
		return ResolvedType{}
	}
	expanded := internaltypes.ExpandAliases(parsed, aliases)
	if expanded == nil {
		return ResolvedType{}
	}
	rt := resolvedTypeFromDocType(*expanded)
	return r.resolveResolvedType(rt, file)
}

func (r *Resolver) expandDocTypeString(raw string, parsed *internaltypes.Type, file *parser.FileNode, aliases map[string]*internaltypes.Type) string {
	if parsed == nil {
		parsed = internaltypes.Parse(raw)
	}
	if parsed == nil {
		return raw
	}
	expanded := internaltypes.ExpandAliases(parsed, aliases)
	if expanded == nil {
		return raw
	}
	return expanded.String()
}

func (r *Resolver) docAliasesForSymbol(file *parser.FileNode, sym *symbolsAliasOwner, doc *parser.DocBlock) map[string]*internaltypes.Type {
	aliases := map[string]*internaltypes.Type{}
	if sym != nil && sym.FQN != "" {
		r.mergeDocAliases(aliases, file, sym.FQN, sym.DocComment, map[string]bool{})
	}
	r.mergeParsedDocAliases(aliases, file, doc, map[string]bool{})
	if len(aliases) == 0 {
		return nil
	}
	return aliases
}

type symbolsAliasOwner struct {
	FQN        string
	DocComment string
}

func (r *Resolver) mergeDocAliases(dst map[string]*internaltypes.Type, file *parser.FileNode, ownerFQN, rawDoc string, seen map[string]bool) {
	if rawDoc == "" {
		return
	}
	if ownerFQN != "" {
		if seen[ownerFQN] {
			return
		}
		seen[ownerFQN] = true
		defer delete(seen, ownerFQN)
	}
	r.mergeParsedDocAliases(dst, file, parser.ParseDocBlock(rawDoc), seen)
}

func (r *Resolver) mergeParsedDocAliases(dst map[string]*internaltypes.Type, file *parser.FileNode, doc *parser.DocBlock, seen map[string]bool) {
	if doc == nil {
		return
	}
	for _, alias := range doc.TypeAliases {
		if alias.Name == "" || alias.ParsedType == nil {
			continue
		}
		dst[alias.Name] = alias.ParsedType
	}
	for _, imported := range doc.ImportedTypes {
		fromFQN := r.ResolveClassName(strings.TrimPrefix(imported.From, "\\"), file)
		if fromFQN == "" || seen[fromFQN] {
			continue
		}
		src := r.Index.Lookup(fromFQN)
		if src == nil || src.DocComment == "" {
			continue
		}
		importedDoc := parser.ParseDocBlock(src.DocComment)
		if importedDoc == nil {
			continue
		}
		nested := map[string]*internaltypes.Type{}
		r.mergeDocAliases(nested, file, fromFQN, src.DocComment, seen)
		for _, alias := range importedDoc.TypeAliases {
			if alias.Name != imported.Name || alias.ParsedType == nil {
				continue
			}
			name := imported.Name
			if imported.Alias != "" {
				name = imported.Alias
			}
			dst[name] = internaltypes.ExpandAliases(alias.ParsedType, nested)
		}
	}
}

func docAliasOwnerFromSymbol(sym *symbols.Symbol) *symbolsAliasOwner {
	if sym == nil {
		return nil
	}
	return &symbolsAliasOwner{FQN: sym.FQN, DocComment: sym.DocComment}
}

func classAliasOwnerAtLine(file *parser.FileNode, line int) *symbolsAliasOwner {
	if file == nil {
		return nil
	}
	for _, cls := range file.Classes {
		if line >= cls.StartLine {
			return &symbolsAliasOwner{
				FQN:        BuildFQN(file.Namespace, cls.Name),
				DocComment: cls.DocComment,
			}
		}
	}
	return nil
}
