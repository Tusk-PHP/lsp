package laravel

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
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

// SyntaxOf answers ValidationSyntax for src from the index, the resolver
// and the project's rule helper methods.
func (v *ValidationRules) SyntaxOf(r *resolve.Resolver, src string, file *parser.FileNode) ValidationSyntax {
	var starts []int
	index := v.index
	return ValidationSyntax{
		ClassName: func(name string) string { return r.ResolveClassName(name, file) },
		ExprClass: func(start, end uint32) string {
			if starts == nil {
				starts = lineStarts(src)
			}
			return r.ChainType(src[start:end], src, offsetPosition(starts, int(end)), file).BaseFQN()
		},
		IsSubtype: index.IsSubtypeOf,
		Helper: func(class, method string) (RulesForm, bool) {
			return v.helpers.Form(index, class, method)
		},
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

// MayHoldRules reports whether a file can hold rule strings worth parsing
// it for: its tokens name a call or method that takes rules (MayValidate),
// or the cursor is in a method of class whose returned value the project
// uses as rules.
func (v *ValidationRules) MayHoldRules(tokens []parser.Token, class, method string) bool {
	if v == nil {
		return false
	}
	if MayValidate(tokens) {
		return true
	}
	_, ok := v.helpers.Form(v.index, class, method)
	return ok
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
