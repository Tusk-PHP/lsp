package rule

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"
)

func declaredCodes(t *testing.T) []Code {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "rule.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var codes []Code
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			if ident, ok := value.Type.(*ast.Ident); !ok || ident.Name != "Code" {
				continue
			}
			literal, err := strconv.Unquote(value.Values[0].(*ast.BasicLit).Value)
			if err != nil {
				t.Fatal(err)
			}
			codes = append(codes, Code(literal))
		}
	}
	return codes
}

func TestAllListsEveryDeclaredCodeOnce(t *testing.T) {
	declared := declaredCodes(t)
	if len(declared) != len(All) {
		t.Errorf("rule.go declares %d codes, All lists %d", len(declared), len(All))
	}
	for _, code := range declared {
		if !slices.Contains(All, code) {
			t.Errorf("%q is declared but missing from All, so config can't name it", code)
		}
	}
	seen := map[Code]bool{}
	for _, code := range All {
		if seen[code] {
			t.Errorf("%q is listed twice in All", code)
		}
		seen[code] = true
	}
}

func TestParseCode(t *testing.T) {
	if code, err := ParseCode("unknown-member"); err != nil || code != UnknownMember {
		t.Errorf("got %q, %v", code, err)
	}
	if _, err := ParseCode("unknown-members"); err == nil {
		t.Error("an unknown code must not parse")
	}
}

func TestSeverityNamesRoundTrip(t *testing.T) {
	for _, severity := range Severities {
		parsed, err := ParseSeverity(severity.String())
		if err != nil || parsed != severity {
			t.Errorf("%d: %q parsed to %d, %v", severity, severity.String(), parsed, err)
		}
	}
	if len(severityNames) != len(Severities) {
		t.Errorf("%d severities have names, %d are listed", len(severityNames), len(Severities))
	}
	if _, err := ParseSeverity("loud"); err == nil {
		t.Error("an unknown severity must not parse")
	}
}

func TestTargets(t *testing.T) {
	cases := []struct {
		text, source, code string
		matches            bool
	}{
		{"unknown-member", "tusk-php", "unknown-member", true},
		{"unknown-member", "phpstan", "unknown-member", false},
		{"phpstan:argument.type", "phpstan", "argument.type", true},
		{"phpstan:argument.type", "phpstan", "return.type", false},
		{"phpstan", "phpstan", "return.type", true},
		{"pint", "phpstan", "braces", false},
	}
	for _, c := range cases {
		target, err := ParseTarget(c.text)
		if err != nil {
			t.Fatalf("%s: %v", c.text, err)
		}
		if target.String() != c.text {
			t.Errorf("%s prints as %q", c.text, target.String())
		}
		if got := target.Matches(c.source, c.code); got != c.matches {
			t.Errorf("%s on %s:%s: got %v", c.text, c.source, c.code, got)
		}
	}
	for _, bad := range []string{"unknown-members", "psalm:x", "psalm"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Errorf("%q must not parse", bad)
		}
	}
}

func TestTargetOf(t *testing.T) {
	if target, err := TargetOf("tusk-php", "unknown-member"); err != nil || target != NativeTarget(UnknownMember) {
		t.Errorf("native: %v, %v", target, err)
	}
	if target, err := TargetOf("phpstan", "argument.type"); err != nil || target.String() != "phpstan:argument.type" {
		t.Errorf("external: %v, %v", target, err)
	}
	for _, bad := range [][2]string{{"tusk-php", "nope"}, {"psalm", "x"}} {
		if _, err := TargetOf(bad[0], bad[1]); err == nil {
			t.Errorf("%v must not make a target", bad)
		}
	}
}
