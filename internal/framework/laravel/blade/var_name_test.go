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

func TestDefinedViewMakesGlobalOnlyValidVariableNames(t *testing.T) {
	doc := CompileWith("@inject('my-service', 'App\\\\Service')\n@inject('metrics', 'App\\\\Metrics')\n{{ $größe }}", CompileOptions{
		Vars:    []Var{{Name: "größe", Type: "int"}, {Name: "bad-name", Type: "int"}, {Name: "9lives", Type: "int"}, {Name: "größe", Type: "int"}},
		Defined: true,
	})
	header, _, _ := strings.Cut(doc.PHP, "\n")
	_, globals, ok := strings.Cut(header, " global ")
	if !ok {
		t.Fatalf("a view with every variable known makes them global: %q", header)
	}
	if want := "$größe, $loop"; !strings.HasPrefix(globals, want) {
		t.Errorf("globals = %q, want them to start with %q", globals, want)
	}
	if !strings.HasSuffix(globals, "$metrics;") {
		t.Errorf("an injected service is global: %q", globals)
	}
	for _, bad := range []string{"$bad", "$9lives", "$my-service"} {
		if strings.Contains(globals, bad) {
			t.Errorf("%s is not a variable name and is not made global: %q", bad, globals)
		}
	}
}
