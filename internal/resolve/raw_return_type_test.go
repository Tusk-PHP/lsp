package resolve

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/symbols"
)

// setupRawReturnResolver builds a minimal resolver with a single class whose
// methods carry various combinations of native type hints and @return docblocks.
func setupRawReturnResolver(t *testing.T) (*Resolver, map[string]*symbols.Symbol) {
	t.Helper()

	src := `<?php
namespace App;

class Other {}
class Parent {}

class Subject {
    /** @return Other */
    public function withObject(): object {}

    /** @return Other */
    public function withMixed(): mixed {}

    /** @return Other */
    public function withSpecificNative(): Parent {}

    public function withObjectNoDoc(): object {}

    public function withMixedNoDoc(): mixed {}
}
`

	idx := symbols.NewIndex()
	idx.IndexFile("file:///raw_return.php", src)
	r := NewResolver(idx)

	lookup := func(name string) *symbols.Symbol {
		m := r.FindMember("App\\Subject", name)
		if m == nil {
			t.Fatalf("method App\\Subject::%s not found in index", name)
		}
		return m
	}

	members := map[string]*symbols.Symbol{
		"withObject":         lookup("withObject"),
		"withMixed":          lookup("withMixed"),
		"withSpecificNative": lookup("withSpecificNative"),
		"withObjectNoDoc":    lookup("withObjectNoDoc"),
		"withMixedNoDoc":     lookup("withMixedNoDoc"),
	}
	return r, members
}

// TestRawReturnDocWinsOverObject: @return SomeClass beats `: object`.
// rawMemberReturnType returns the raw (unexpanded) type string, so we expect
// the unqualified name as it appears in the docblock.
func TestRawReturnDocWinsOverObject(t *testing.T) {
	r, members := setupRawReturnResolver(t)
	got := r.rawMemberReturnType(members["withObject"])
	if got != "Other" {
		t.Fatalf("withObject: rawMemberReturnType = %q, want Other", got)
	}
}

// TestRawReturnDocWinsOverMixed: @return SomeClass beats `: mixed`.
func TestRawReturnDocWinsOverMixed(t *testing.T) {
	r, members := setupRawReturnResolver(t)
	got := r.rawMemberReturnType(members["withMixed"])
	if got != "Other" {
		t.Fatalf("withMixed: rawMemberReturnType = %q, want Other", got)
	}
}

// TestRawReturnNativeWinsOverDoc: a specific native hint like `: Parent` is
// NOT overridden by the @return docblock — the native hint stays authoritative.
func TestRawReturnNativeWinsOverDoc(t *testing.T) {
	r, members := setupRawReturnResolver(t)
	got := r.rawMemberReturnType(members["withSpecificNative"])
	if got != "Parent" {
		t.Fatalf("withSpecificNative: rawMemberReturnType = %q, want Parent", got)
	}
}

// TestRawReturnObjectNativeNoDoc: `: object` with no docblock still returns
// "object" — no regression for the bare-type case.
func TestRawReturnObjectNativeNoDoc(t *testing.T) {
	r, members := setupRawReturnResolver(t)
	got := r.rawMemberReturnType(members["withObjectNoDoc"])
	if got != "object" {
		t.Fatalf("withObjectNoDoc: rawMemberReturnType = %q, want object", got)
	}
}

// TestRawReturnMixedNativeNoDoc: `: mixed` with no docblock still returns
// "mixed" — no regression for the bare-type case.
func TestRawReturnMixedNativeNoDoc(t *testing.T) {
	r, members := setupRawReturnResolver(t)
	got := r.rawMemberReturnType(members["withMixedNoDoc"])
	if got != "mixed" {
		t.Fatalf("withMixedNoDoc: rawMemberReturnType = %q, want mixed", got)
	}
}
