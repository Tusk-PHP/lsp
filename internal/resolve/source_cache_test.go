package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func hasBinding(r *Resolver, lines []string, name string) bool {
	for _, s := range r.ScopeDocument(lines).Scopes {
		for _, b := range s.Bindings {
			if b.Name == name {
				return true
			}
		}
	}
	return false
}

func TestScopeDocumentIsAlwaysTheOneOfTheLinesPassed(t *testing.T) {
	before := "<?php\nfunction run() {\n    $a = 1;\n}\n"
	after := strings.Replace(before, "$a = 1;", "$a = 1;\n    $b = 2;", 1)
	r := NewResolver(symbols.NewIndex())

	issued := r.Lines(before)
	if again := r.Lines(before); &again[0] != &issued[0] {
		t.Fatal("the same text gets new lines")
	}
	doc := r.ScopeDocument(issued)
	if r.ScopeDocument(issued) != doc || r.ScopeDocument(SplitLines(before)) != doc {
		t.Error("the same text, from the resolver's lines or another split of it, collects its scopes again")
	}
	if hasBinding(r, issued, "$b") {
		t.Fatal("$b before it is written")
	}

	edited := r.Lines(after)
	if !hasBinding(r, edited, "$b") || r.ScopeDocument(edited) == doc {
		t.Error("after the edit, the scopes are still the ones of the text before it")
	}
	if hasBinding(r, issued, "$b") {
		t.Error("the lines from before the edit get the scopes of the edited text")
	}
	if !hasBinding(r, SplitLines(after), "$b") {
		t.Error("another split of the edited text gets the scopes of the text before it")
	}
	renamed := strings.Replace(before, "$a = 1;", "$b = 1;", 1)
	if !hasBinding(r, SplitLines(renamed), "$b") || hasBinding(r, issued, "$b") {
		t.Error("an edit that keeps the line count shares the scopes of the text before it")
	}
	pos := textpos.Position{Line: 3, Character: 4}
	if !r.IsLocalVariable("$b", edited, pos) || r.IsLocalVariable("$b", issued, pos) {
		t.Error("a variable's scope follows the cached text, not the lines passed")
	}
}
