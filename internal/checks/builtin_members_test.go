package checks

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func unknownMembersOn(t *testing.T, typ, call string) []Finding {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.4"})
	idx.MarkReady()
	src := "<?php\nfunction f(\\" + typ + " $it): void {\n    $it->" + call + ";\n}\n"
	rule := &UnknownMemberRule{TypeResolver: func(expr, _ string, _, _ int, _ *parser.FileNode) string {
		if expr == "$it" {
			return typ
		}
		return ""
	}}
	return rule.Check(parser.ParseFile(src), src, idx)
}

func TestUnknownMemberStaysQuietOnBuiltinsKnownOnlyFromTheGeneratedTable(t *testing.T) {
	for _, f := range unknownMembersOn(t, "DOMNode", "noSuchMethod()") {
		t.Errorf("DOMNode is known from the runtimes the table was generated from, which can't show a member is missing: %s", f.Message)
	}
}

func TestUnknownMemberKnowsTheGeneratedMembersOfAHandStubbedBuiltin(t *testing.T) {
	for _, f := range unknownMembersOn(t, "Fiber", "throw(new \\Exception())") {
		t.Errorf("Fiber::throw, which only the generated table lists, exists: %s", f.Message)
	}
}

func TestUnknownMemberStillReportsOnHandStubbedBuiltins(t *testing.T) {
	if got := unknownMembersOn(t, "DateTime", "noSuchMethod()"); len(got) != 1 {
		t.Errorf("a class its hand-written stub declares in full still shows a missing member: %+v", got)
	}
}
