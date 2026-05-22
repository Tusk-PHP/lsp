package scope

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

func TestCollectVariablesAndParameters(t *testing.T) {
	source := `<?php
function run($name) {
    $local = $name;
    return $local;
}
`

	doc := Collect(source)

	nameBinding := doc.BindingAt(protocol.Position{Line: 1, Character: 14})
	if nameBinding == nil {
		t.Fatal("expected parameter binding at $name")
	}
	if nameBinding.Kind != BindingParameter {
		t.Fatalf("parameter binding kind = %q", nameBinding.Kind)
	}
	if got := len(doc.Occurrences(nameBinding)); got != 2 {
		t.Fatalf("expected 2 occurrences for $name, got %d", got)
	}

	localBinding := doc.BindingAt(protocol.Position{Line: 2, Character: 5})
	if localBinding == nil {
		t.Fatal("expected local binding at $local")
	}
	if localBinding.Kind != BindingVariable {
		t.Fatalf("local binding kind = %q", localBinding.Kind)
	}
	if got := len(doc.Occurrences(localBinding)); got != 2 {
		t.Fatalf("expected 2 occurrences for $local, got %d", got)
	}
}

func TestCollectForeachAndCatchBindings(t *testing.T) {
	source := `<?php
function run(array $rows) {
    foreach ($rows as $key => [$left, $right]) {
        echo $key;
        echo $left;
        echo $right;
    }

    try {
    } catch (\Throwable $e) {
        echo $e;
    }
}
`

	doc := Collect(source)

	for _, tc := range []struct {
		name string
		pos  protocol.Position
		kind BindingKind
		occ  int
	}{
		{name: "$key", pos: protocol.Position{Line: 2, Character: 23}, kind: BindingForeachKey, occ: 2},
		{name: "$left", pos: protocol.Position{Line: 2, Character: 32}, kind: BindingDestructure, occ: 2},
		{name: "$right", pos: protocol.Position{Line: 2, Character: 39}, kind: BindingDestructure, occ: 2},
	} {
		binding := doc.BindingAt(tc.pos)
		if binding == nil {
			t.Fatalf("expected binding for %s", tc.name)
		}
		if binding.Kind != tc.kind {
			t.Fatalf("%s kind = %q, want %q", tc.name, binding.Kind, tc.kind)
		}
		if got := len(doc.Occurrences(binding)); got != tc.occ {
			t.Fatalf("%s occurrences = %d, want %d", tc.name, got, tc.occ)
		}
	}

	var catchBinding *Binding
	for _, binding := range doc.Bindings {
		if binding.Name == "$e" {
			catchBinding = binding
			break
		}
	}
	if catchBinding == nil {
		t.Fatal("expected catch binding for $e")
	}
	if catchBinding.Kind != BindingCatch {
		t.Fatalf("$e kind = %q, want %q", catchBinding.Kind, BindingCatch)
	}
	if got := len(doc.Occurrences(catchBinding)); got != 2 {
		t.Fatalf("$e occurrences = %d, want 2", got)
	}
}

func TestCollectImportsAndClosureUse(t *testing.T) {
	source := `<?php
use App\Support\Helper as Tool;

function run($value) {
    $prefix = "hi";
    $fn = function ($name) use ($prefix) {
        return $prefix . $name;
    };

    return $fn($value);
}
`

	doc := Collect(source)

	var importBinding *Binding
	for _, binding := range doc.Bindings {
		if binding.Kind == BindingImport && binding.Name == "Tool" {
			importBinding = binding
			break
		}
	}
	if importBinding == nil {
		t.Fatal("expected file import binding for Tool")
	}

	prefixBinding := doc.BindingAt(protocol.Position{Line: 4, Character: 6})
	if prefixBinding == nil || prefixBinding.Kind != BindingVariable {
		t.Fatal("expected outer variable binding for $prefix")
	}

	closureUse := doc.BindingAt(protocol.Position{Line: 5, Character: 32})
	if closureUse == nil {
		t.Fatal("expected closure-use binding for $prefix")
	}
	if closureUse.Kind != BindingClosureUse {
		t.Fatalf("closure use kind = %q", closureUse.Kind)
	}
	if closureUse.Origin != prefixBinding {
		t.Fatal("expected closure-use binding to link back to outer variable")
	}

	if got := len(doc.Occurrences(prefixBinding)); got != 3 {
		t.Fatalf("expected grouped outer+closure occurrences for $prefix, got %d", got)
	}
}

func TestCollectArrowFunctionCaptures(t *testing.T) {
	source := `<?php
function run(array $rows) {
    $prefix = "user";
    $mapper = fn ($row) => $prefix . $row;
    return $mapper($rows);
}
`

	doc := Collect(source)

	prefixBinding := findBinding(doc, "$prefix", BindingVariable)
	if prefixBinding == nil {
		t.Fatal("expected outer $prefix binding")
	}
	capture := findBinding(doc, "$prefix", BindingClosureUse)
	if capture == nil {
		t.Fatal("expected arrow-function capture for $prefix")
	}
	if capture.Origin != prefixBinding {
		t.Fatal("expected arrow-function capture to link to outer $prefix")
	}
	if got := len(doc.Occurrences(prefixBinding)); got != 2 {
		t.Fatalf("expected grouped outer+arrow occurrences for $prefix, got %d", got)
	}

	rowBinding := findBinding(doc, "$row", BindingParameter)
	if rowBinding == nil {
		t.Fatal("expected arrow-function parameter binding for $row")
	}
	if got := len(doc.Occurrences(rowBinding)); got != 2 {
		t.Fatalf("expected declaration and reference for $row, got %d", got)
	}
}

func findBinding(doc *Document, name string, kind BindingKind) *Binding {
	for _, binding := range doc.Bindings {
		if binding.Name == name && binding.Kind == kind {
			return binding
		}
	}
	return nil
}

func TestCollectGroupedImportsAndArrowCaptures(t *testing.T) {
	source := `<?php
use App\Support\{Helper, Tool as Alias};

function run($value) {
    $prefix = "hi";
    $fn = fn($name) => $prefix . $name . $value;

    return $fn($value);
}
`

	doc := Collect(source)

	var helperImport *Binding
	var aliasImport *Binding
	for _, binding := range doc.Bindings {
		if binding.Kind != BindingImport {
			continue
		}
		switch binding.Name {
		case "Helper":
			helperImport = binding
		case "Alias":
			aliasImport = binding
		}
	}
	if helperImport == nil {
		t.Fatal("expected grouped use import binding for Helper")
	}
	if aliasImport == nil {
		t.Fatal("expected grouped use import binding for Alias")
	}

	prefixBinding := doc.BindingAt(protocol.Position{Line: 4, Character: 6})
	if prefixBinding == nil || prefixBinding.Kind != BindingVariable {
		t.Fatal("expected outer variable binding for $prefix")
	}

	arrowCapture := doc.BindingAt(protocol.Position{Line: 5, Character: 26})
	if arrowCapture == nil {
		t.Fatal("expected arrow capture binding for $prefix")
	}
	if arrowCapture.Kind != BindingClosureUse {
		t.Fatalf("arrow capture kind = %q, want %q", arrowCapture.Kind, BindingClosureUse)
	}
	if arrowCapture.Origin != prefixBinding {
		t.Fatal("expected arrow capture to link back to outer variable")
	}

	valueBinding := doc.BindingAt(protocol.Position{Line: 3, Character: 14})
	if valueBinding == nil || valueBinding.Kind != BindingParameter {
		t.Fatal("expected parameter binding for $value")
	}
	if got := len(doc.Occurrences(valueBinding)); got != 3 {
		t.Fatalf("expected 3 occurrences for captured parameter $value, got %d", got)
	}
}

func TestCollectListDestructureBindings(t *testing.T) {
	source := `<?php
function run() {
    list($first, $second) = [1, 2];
    echo $first;
    echo $second;
}
`

	doc := Collect(source)

	for _, tc := range []struct {
		name string
		pos  protocol.Position
	}{
		{name: "$first", pos: protocol.Position{Line: 2, Character: 9}},
		{name: "$second", pos: protocol.Position{Line: 2, Character: 17}},
	} {
		binding := doc.BindingAt(tc.pos)
		if binding == nil {
			t.Fatalf("expected binding for %s", tc.name)
		}
		if binding.Kind != BindingDestructure {
			t.Fatalf("%s kind = %q, want %q", tc.name, binding.Kind, BindingDestructure)
		}
		if got := len(doc.Occurrences(binding)); got != 2 {
			t.Fatalf("%s occurrences = %d, want 2", tc.name, got)
		}
	}
}

func TestCollectVariableAssignments(t *testing.T) {
	source := `<?php
function run($name, $flag) {
    $label = $name;
    if ($flag) {
        $label = strtoupper($label);
    }
    return $label;
}
`

	doc := Collect(source)
	label := doc.BindingAt(protocol.Position{Line: 2, Character: 5})
	if label == nil {
		t.Fatal("expected binding for $label")
	}
	if got := len(label.Assignments); got != 2 {
		t.Fatalf("expected 2 assignments, got %d", got)
	}
	if !label.Assignments[0].DirectStatement {
		t.Fatal("expected the first assignment to be a direct statement")
	}
	if got := label.Assignments[0].RelativeBraceDepth; got != 0 {
		t.Fatalf("expected top-level assignment depth 0, got %d", got)
	}
	if got := label.Assignments[1].RelativeBraceDepth; got != 1 {
		t.Fatalf("expected nested assignment depth 1, got %d", got)
	}
}
