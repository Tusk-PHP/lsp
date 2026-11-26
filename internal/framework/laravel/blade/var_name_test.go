package blade

import (
	"strings"
	"testing"
)

func TestCompiledViewDeclaresOnlyValidVariableNames(t *testing.T) {
	doc := CompileWithVars("{{ $größe }}", []Var{{Name: "größe", Type: "int"}, {Name: "bad-name", Type: "int"}, {Name: "9lives", Type: "int"}})
	if !strings.Contains(doc.PHP, "@var int $größe */") {
		t.Errorf("a non-ASCII name is declared: %q", doc.PHP)
	}
	for _, bad := range []string{"$bad", "$9lives"} {
		if strings.Contains(doc.PHP, bad) {
			t.Errorf("%s is not a variable name and is not declared: %q", bad, doc.PHP)
		}
	}
}
