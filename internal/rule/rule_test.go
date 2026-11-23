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
	for _, severity := range []Severity{SeverityError, SeverityWarning, SeverityInfo, SeverityHint} {
		parsed, err := ParseSeverity(severity.String())
		if err != nil || parsed != severity {
			t.Errorf("%d: %q parsed to %d, %v", severity, severity.String(), parsed, err)
		}
	}
	if _, err := ParseSeverity("loud"); err == nil {
		t.Error("an unknown severity must not parse")
	}
}
