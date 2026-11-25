package symbols

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

type traitRules struct {
	owner    string
	excluded map[string]bool
	aliases  []traitAlias
}

type traitAlias struct {
	trait      string
	method     string
	alias      string
	visibility string
	final      bool
}

func traitMethodKey(trait, method string) string {
	return strings.ToLower(trait) + "::" + strings.ToLower(method)
}

func newTraitRules(owner string, adaptations parser.TraitAdaptations, resolve func(string) string) *traitRules {
	if len(adaptations.Precedences) == 0 && len(adaptations.Aliases) == 0 {
		return nil
	}
	rules := &traitRules{owner: owner}
	for _, p := range adaptations.Precedences {
		if rules.excluded == nil {
			rules.excluded = make(map[string]bool, len(p.InsteadOf))
		}
		for _, loser := range p.InsteadOf {
			rules.excluded[traitMethodKey(resolve(loser), p.Method)] = true
		}
	}
	rules.aliases = make([]traitAlias, 0, len(adaptations.Aliases))
	for _, a := range adaptations.Aliases {
		alias := traitAlias{method: a.Method, alias: a.Alias, visibility: a.Visibility, final: a.Final}
		if a.Trait != "" {
			alias.trait = resolve(a.Trait)
		}
		rules.aliases = append(rules.aliases, alias)
	}
	return rules
}

func (idx *Index) traitRulesLocked(fqn string) *traitRules {
	if sym := idx.symbols[fqn]; sym != nil {
		return sym.traitRules
	}
	return nil
}

func (r *traitRules) excludes(trait, method string) bool {
	return r != nil && r.excluded[traitMethodKey(trait, method)]
}

func (a traitAlias) applies(trait string, method *Symbol) bool {
	return strings.EqualFold(a.method, method.Name) &&
		(a.trait == "" || strings.EqualFold(a.trait, trait))
}

func (r *traitRules) adapt(trait string, method *Symbol) (kept *Symbol, aliases []*Symbol) {
	if r == nil || method.Kind != KindMethod {
		return method, nil
	}
	kept = method
	for _, a := range r.aliases {
		if !a.applies(trait, method) {
			continue
		}
		if a.alias == "" {
			kept = a.modified(kept)
			continue
		}
		renamed := *a.modified(method)
		renamed.Name = a.alias
		renamed.FQN = r.owner + "::" + a.alias
		renamed.ParentFQN = r.owner
		renamed.AliasOf = method
		aliases = append(aliases, &renamed)
	}
	if r.excludes(trait, method.Name) {
		kept = nil
	}
	return kept, aliases
}

func (a traitAlias) modified(method *Symbol) *Symbol {
	visibility := method.Visibility
	if a.visibility != "" {
		visibility = a.visibility
	}
	final := method.IsFinal || a.final
	if visibility == method.Visibility && final == method.IsFinal {
		return method
	}
	copied := *method
	copied.Visibility, copied.IsFinal = visibility, final
	return &copied
}

func (idx *Index) aliasedMethodLocked(owner *Symbol, name string) *Symbol {
	if owner.traitRules == nil {
		return nil
	}
	for _, a := range owner.traitRules.aliases {
		if !strings.EqualFold(a.alias, name) {
			continue
		}
		for _, trait := range idx.traitMap[owner.FQN] {
			if a.trait != "" && !strings.EqualFold(a.trait, trait) {
				continue
			}
			if m := methodNamed(idx.getClassMembersRecursive(trait, map[string]bool{owner.FQN: true}), a.method); m != nil {
				return a.modified(m)
			}
		}
	}
	return nil
}
