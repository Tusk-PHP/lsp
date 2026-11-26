package diagnostics

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/checks"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// NewIndexMemberChecker returns a MemberChecker backed by the symbol index.
// liveSchema reports whether the models' database columns were read from a
// live database; only then is a model's column list known to be complete.
// It may be nil.
func NewIndexMemberChecker(index *symbols.Index, liveSchema func() bool) checks.MemberChecker {
	return &indexMemberChecker{index: index, liveSchema: liveSchema}
}

// indexMemberChecker implements checks.MemberChecker using the symbol index.
type indexMemberChecker struct {
	index      *symbols.Index
	liveSchema func() bool
}

// KnowsColumns reports whether model's columns were read from a live
// database. Columns worked out from migrations, docblocks or an IDE helper
// file can be incomplete, and a name missing from them proves nothing.
func (c *indexMemberChecker) KnowsColumns(modelFQN string) bool {
	if c.liveSchema == nil || !c.liveSchema() {
		return false
	}
	return slices.ContainsFunc(c.index.GetClassMembers(modelFQN), frameworklaravel.IsLiveDBColumnProperty)
}

// KnowsRelations reports whether every class and trait model is made of is
// indexed and none of them but Eloquent's Model handles calls to undeclared
// methods, so a relation method missing from the index is really missing.
func (c *indexMemberChecker) KnowsRelations(modelFQN string) bool {
	classes := append([]string{modelFQN}, c.index.GetInheritanceChain(modelFQN)...)
	if classes[len(classes)-1] != frameworklaravel.EloquentModel {
		return false // an ancestor isn't indexed
	}
	seen := map[string]bool{}
	for len(classes) > 0 {
		fqn := classes[0]
		classes = classes[1:]
		if seen[fqn] {
			continue
		}
		seen[fqn] = true
		if fqn == frameworklaravel.EloquentModel {
			continue
		}
		if c.index.Lookup(fqn) == nil || c.index.Lookup(fqn+"::__call") != nil {
			return false
		}
		classes = append(classes, c.index.TraitsOf(fqn)...)
	}
	return true
}

func (c *indexMemberChecker) IsColumn(modelFQN, name string) bool {
	return c.hasProperty(modelFQN, name, frameworklaravel.IsColumnProperty)
}

func (c *indexMemberChecker) IsDBColumn(modelFQN, name string) bool {
	return c.hasProperty(modelFQN, name, frameworklaravel.IsDBColumnProperty)
}

func (c *indexMemberChecker) hasProperty(modelFQN, name string, is func(*symbols.Symbol) bool) bool {
	return slices.ContainsFunc(c.index.GetClassMembers(modelFQN), func(m *symbols.Symbol) bool {
		return strings.TrimPrefix(m.Name, "$") == name && is(m)
	})
}

func (c *indexMemberChecker) RelatedModelFQN(modelFQN, relationName string) string {
	return frameworklaravel.RelatedModel(c.index, modelFQN, relationName)
}

// IsRelation reports whether model has a method that can be the relation
// name. Any method of that name counts: a relation needs no return type, and
// one that returns something else is not this rule's to judge. PHP method
// names ignore case.
func (c *indexMemberChecker) IsRelation(modelFQN, name string) bool {
	for _, m := range c.index.GetClassMembers(modelFQN) {
		if m.Kind == symbols.KindMethod && strings.EqualFold(m.Name, name) {
			return true
		}
	}
	return false
}
