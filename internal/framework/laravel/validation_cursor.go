package laravel

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// ValidationRuleSlot is the rule under the cursor in a validation rule
// string.
type ValidationRuleSlot struct {
	// Name is the rule's name as written.
	Name string
	// Typed is the part of the name before the cursor.
	Typed string
	// InName says whether the cursor is in the name rather than in its
	// parameters.
	InName bool
	// HasParameters says whether a `:` follows the name.
	HasParameters bool
	// NameRange is the name's range, TypedRange from its start to the cursor.
	NameRange, TypedRange textpos.Range
}

// ValidationSyntaxOf answers ValidationSyntax for src from the index and
// the resolver.
func ValidationSyntaxOf(index *symbols.Index, r *resolve.Resolver, src string, file *parser.FileNode) ValidationSyntax {
	var starts []int
	return ValidationSyntax{
		ClassName: func(name string) string { return r.ResolveClassName(name, file) },
		ExprClass: func(start, end uint32) string {
			if starts == nil {
				starts = lineStarts(src)
			}
			return r.ChainType(src[start:end], src, offsetPosition(starts, int(end)), file).BaseFQN()
		},
		IsSubtype: index.IsSubtypeOf,
	}
}

// MayValidate reports whether a file's tokens name a call or method that
// takes validation rules, so a file without one is never parsed for them.
func MayValidate(tokens []parser.Token) bool {
	for _, t := range tokens {
		if t.Kind != parser.TokenIdentifier {
			continue
		}
		switch strings.ToLower(t.Value) {
		case "validate", "validatewithbag", "validator", "rules":
			return true
		}
	}
	return false
}

// ValidationRuleSlotAt returns the rule under the cursor when it is in a
// validation rule string.
func ValidationRuleSlotAt(src string, pos textpos.Position, syn ValidationSyntax) (ValidationRuleSlot, bool) {
	starts := lineStarts(src)
	if pos.Line < 0 || pos.Line >= len(starts) {
		return ValidationRuleSlot{}, false
	}
	offset := starts[pos.Line] + pos.Character
	if offset > len(src) {
		return ValidationRuleSlot{}, false
	}
	_, seg, ok := ValidationRuleAt(src, phpparser.Parse(src, token.Latest), syn, uint32(offset))
	if !ok {
		return ValidationRuleSlot{}, false
	}
	at := uint32(offset)
	slot := ValidationRuleSlot{
		Name:          seg.Name(src),
		InName:        at <= seg.NameEnd,
		HasParameters: seg.NameEnd < seg.End,
		NameRange:     textpos.Range{Start: offsetPosition(starts, int(seg.Start)), End: offsetPosition(starts, int(seg.NameEnd))},
	}
	if slot.InName {
		slot.Typed = src[seg.Start:at]
		slot.TypedRange = textpos.Range{Start: slot.NameRange.Start, End: pos}
	}
	return slot, true
}
