package diagnostics

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/checks"
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

// databaseColumnDoc starts the doc comment of a column read from the
// database schema.
const databaseColumnDoc = "(database column)"

// KnowsColumns reports whether model's columns were read from a live
// database. Columns worked out from migrations, docblocks or an IDE helper
// file can be incomplete, and a name missing from them proves nothing.
func (c *indexMemberChecker) KnowsColumns(modelFQN string) bool {
	if c.liveSchema == nil || !c.liveSchema() {
		return false
	}
	for _, m := range c.index.GetClassMembers(modelFQN) {
		if m.Kind == symbols.KindProperty && m.IsVirtual && strings.HasPrefix(m.DocComment, databaseColumnDoc) {
			return true
		}
	}
	return false
}

// KnowsRelations reports whether every class and trait model is made of is
// indexed and none of them but Eloquent's Model handles calls to undeclared
// methods, so a relation method missing from the index is really missing.
func (c *indexMemberChecker) KnowsRelations(modelFQN string) bool {
	classes := append([]string{modelFQN}, c.index.GetInheritanceChain(modelFQN)...)
	if classes[len(classes)-1] != eloquentBaseModel {
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
		if fqn == eloquentBaseModel {
			continue
		}
		if c.index.Lookup(fqn) == nil || c.index.Lookup(fqn+"::__call") != nil {
			return false
		}
		classes = append(classes, c.index.TraitsOf(fqn)...)
	}
	return true
}

const eloquentBaseModel = "Illuminate\\Database\\Eloquent\\Model"

func (c *indexMemberChecker) IsColumn(modelFQN, name string) bool {
	for _, m := range c.index.GetClassMembers(modelFQN) {
		if m.Kind != symbols.KindProperty {
			continue
		}
		colName := strings.TrimPrefix(m.Name, "$")
		if colName != name {
			continue
		}
		// Exclude relation-derived virtual properties
		if m.IsVirtual && strings.HasSuffix(m.DocComment, " relation") {
			continue
		}
		// Exclude accessors
		if m.IsVirtual && strings.HasPrefix(m.DocComment, "Accessor from ") {
			continue
		}
		return true
	}
	return false
}

func (c *indexMemberChecker) IsDBColumn(modelFQN, name string) bool {
	for _, m := range c.index.GetClassMembers(modelFQN) {
		if m.Kind != symbols.KindProperty || !m.IsVirtual {
			continue
		}
		colName := strings.TrimPrefix(m.Name, "$")
		if colName != name {
			continue
		}
		doc := m.DocComment
		if strings.HasPrefix(doc, "From migration") ||
			strings.HasPrefix(doc, databaseColumnDoc) ||
			doc == "Doctrine column" || doc == "Doctrine ID column" {
			return true
		}
		// Skip relations and accessors
		if strings.HasSuffix(doc, " relation") || strings.HasPrefix(doc, "Accessor from ") {
			continue
		}
		// Other virtual properties (@property from IDE helper)
		return true
	}
	return false
}

func (c *indexMemberChecker) RelatedModelFQN(modelFQN, relationName string) string {
	// Look at the virtual property for this relation — for singular relations
	// the Type is "?RelatedModelFQN", for plural it's "Collection" (lost).
	for _, m := range c.index.GetClassMembers(modelFQN) {
		if m.Kind != symbols.KindProperty {
			continue
		}
		propName := strings.TrimPrefix(m.Name, "$")
		if propName != relationName {
			continue
		}
		if !m.IsVirtual || !strings.HasSuffix(m.DocComment, " relation") {
			continue
		}
		typ := m.Type
		// Singular relations: "?App\Models\Product" → "App\Models\Product"
		if strings.HasPrefix(typ, "?") {
			return typ[1:]
		}
		// Plural relations: Type is "Illuminate\Database\Eloquent\Collection"
		// Can't determine related model from this — return ""
		return ""
	}
	return ""
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
