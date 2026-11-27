package parser

import (
	"reflect"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestGrammarRangeParsesWithTheNewestGrammarThatHasNoErrors(t *testing.T) {
	cases := []struct {
		name     string
		grammars GrammarRange
		src      string
		want     token.Version
		errors   bool
	}{
		{"clean on the newest", AnyGrammar(), "<?php #[Marker] class A {}", token.Latest, false},
		{"only PHP 7.4 reads match as a name", AnyGrammar(), "<?php function match() {}", token.PHP74, false},
		{"the newest grammar without errors wins, not the oldest", AnyGrammar(), "<?php class readonly {} function f(int|string $a) {}", token.PHP80, false},
		{"a range without 7.4 keeps the newest", GrammarsBetween(token.PHP80, token.Latest), "<?php function match() {}", token.Latest, true},
		{"one grammar is the only one tried", OnlyGrammar(token.PHP83), "<?php function match() {}", token.PHP83, true},
		{"broken on every grammar keeps the newest", AnyGrammar(), "<?php function match( {", token.Latest, true},
		{"PHP 7.4's unterminated-comment warning is not clean", AnyGrammar(), "<?php /* never closed\nclass A {}", token.Latest, true},
		{"a chosen grammar that parses cleanly is kept", AnyGrammar().After(token.PHP80), "<?php #[Marker] class A {}", token.PHP80, false},
		{"a chosen grammar that fails gives way to a clean newest", AnyGrammar().After(token.PHP74), "<?php #[Marker]\nclass A { function f(int|string $a) {} }", token.Latest, false},
		{"a chosen grammar that fails is kept when the newest fails too", AnyGrammar().After(token.PHP81), "<?php class readonly {} function f(int|string $a) {}", token.PHP81, true},
		{"a chosen newest grammar that fails is kept", AnyGrammar().After(token.Latest), "<?php function match() {}", token.Latest, true},
		{"a choice outside the range is chosen afresh", GrammarsBetween(token.PHP80, token.Latest).After(token.PHP74), "<?php class readonly {} function f(int|string $a) {}", token.PHP80, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree := tc.grammars.Parse(tc.src)
			if tree.Version != tc.want || (len(tree.Errors) > 0) != tc.errors {
				t.Fatalf("Parse = PHP %s with errors %v, want PHP %s with errors: %v", tree.Version, tree.Errors, tc.want, tc.errors)
			}
			if want := Parse(tc.src, tc.want); !reflect.DeepEqual(tree.Errors, want.Errors) {
				t.Errorf("errors = %v, want PHP %s's %v", tree.Errors, tc.want, want.Errors)
			}
		})
	}
}

func TestPHP74WarnsAboutAnUnterminatedComment(t *testing.T) {
	tree := Parse("<?php /* never closed", token.PHP74)
	if len(tree.Errors) != 0 || len(tree.Warnings) != 1 || tree.Warnings[0].Message != "unterminated comment" || tree.Warnings[0].Start != 6 {
		t.Errorf("errors %v, warnings %v, want one unterminated-comment warning at 6", tree.Errors, tree.Warnings)
	}
}

func TestGrammarRangeParseCodeSelectsLikeParse(t *testing.T) {
	if tree := AnyGrammar().ParseCode("function match() {}"); tree.Version != token.PHP74 || len(tree.Errors) > 0 {
		t.Errorf("ParseCode = PHP %s with errors %v, want PHP 7.4 without", tree.Version, tree.Errors)
	}
}

func TestGrammarRange(t *testing.T) {
	if AnyGrammar() != (GrammarRange{}) || AnyGrammar() != GrammarsBetween(token.PHP74, token.Latest) {
		t.Errorf("AnyGrammar() = %s, want every grammar as the zero value", AnyGrammar())
	}
	if GrammarsBetween(token.PHP83, token.PHP80) != GrammarsBetween(token.PHP80, token.PHP83) {
		t.Error("GrammarsBetween depends on the order of its bounds")
	}
	r := GrammarsBetween(token.PHP81, token.PHP84)
	if r.Oldest() != token.PHP81 || r.Newest() != token.PHP84 || r.String() != "8.1-8.4" {
		t.Errorf("GrammarsBetween(8.1, 8.4) = %s (%s to %s)", r, r.Oldest(), r.Newest())
	}
	if !r.Admits(token.PHP81) || !r.Admits(token.PHP84) || r.Admits(token.PHP80) || r.Admits(token.PHP85) {
		t.Errorf("GrammarsBetween(8.1, 8.4) admits the wrong versions")
	}
	if got := OnlyGrammar(token.PHP74).String(); got != "7.4" {
		t.Errorf("OnlyGrammar(7.4) = %s", got)
	}
}
