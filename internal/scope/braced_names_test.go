package scope

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestABracedNameInAStringReadsTheVariable(t *testing.T) {
	for name, literal := range map[string]string{
		"double quotes":     `"${label}"`,
		"string offset":     `"${label['k']}"`,
		"integer offset":    `"${label[0]}"`,
		"followed by text":  `"${label}s"`,
		"heredoc":           "<<<EOT\n${label}\nEOT",
		"shell command":     "`echo ${label}`",
		"after a variable":  `"$other ${label}"`,
		"in braced code":    `"{$other[" ${label} "]}"`,
		"inside a heredoc":  "<<<EOT\n  {$other} ${label}\n  EOT",
		"next to the plain": `"${label}$label"`,
	} {
		t.Run(name, func(t *testing.T) {
			source := "<?php\nfunction run($other) {\n    $label = 'x';\n    return " + literal + ";\n}\n"
			doc := Collect(source)
			label := doc.BindingAt(at(t, source, "$label ="))
			if label == nil {
				t.Fatal("no binding for $label")
			}
			name := at(t, source, "{label")
			name.Character++
			want := textpos.Range{Start: name, End: textpos.Position{Line: name.Line, Character: name.Character + len("label")}}
			var braced []Occurrence
			for _, occurrence := range doc.Occurrences(label) {
				if occurrence.Braced {
					braced = append(braced, occurrence)
				}
			}
			if len(braced) != 1 || braced[0].Range != want {
				t.Fatalf("braced occurrences of $label = %+v, want one at %v", braced, want)
			}
			if doc.BindingAt(name) != label {
				t.Errorf("the name in ${label} is not $label")
			}
			if label.Scope.AccessesVariablesByName {
				t.Errorf("${label} marked %s as reading its variables by name", label.Scope.Name)
			}
		})
	}
}

func TestOccurrenceSpellsTheNameAsWritten(t *testing.T) {
	if got := (Occurrence{}).Spelling("$title"); got != "$title" {
		t.Errorf("plain occurrence spelled %q, want $title", got)
	}
	if got := (Occurrence{Braced: true}).Spelling("$title"); got != "title" {
		t.Errorf("braced occurrence spelled %q, want title", got)
	}
}

func TestAComputedNameInAStringAccessesVariablesByName(t *testing.T) {
	for name, literal := range map[string]string{
		"variable":           `"${$name}"`,
		"constant":           `"${ label }"`,
		"concatenation":      `"${label . 'x'}"`,
		"member access":      "<<<EOT\n${label->name}\nEOT",
		"call":               "`echo ${strtolower($name)}`",
		"in an arrow body":   `(fn() => "${$name}")()`,
		"in a nested string": `"{$other["${$name}"]}"`,
	} {
		t.Run(name, func(t *testing.T) {
			source := "<?php\nfunction run($name, $other) {\n    $label = 'x';\n    return " + literal + ";\n}\n"
			doc := Collect(source)
			label := doc.BindingAt(at(t, source, "$label ="))
			if label == nil {
				t.Fatal("no binding for $label")
			}
			if !label.Scope.AccessesVariablesByName {
				t.Errorf("%s does not read its variables by name", label.Scope.Name)
			}
			if got := len(doc.Occurrences(label)); got != 1 {
				t.Errorf("$label has %d occurrences, want only its declaration", got)
			}
		})
	}
}

func TestAComputedNameReadsTheVariablesItNames(t *testing.T) {
	source := "<?php\nfunction run($name) {\n    return \"${$name}\";\n}\n"
	doc := Collect(source)
	name := doc.BindingAt(at(t, source, "$name)"))
	if name == nil || len(name.References) != 1 || name.References[0].Start != at(t, source, "$name}") {
		t.Fatalf("$name = %+v, want one reference inside ${…}", name)
	}
}

func TestAComputedNameMarksOnlyTheScopeItIsIn(t *testing.T) {
	source := "<?php\nfunction outer($name) {\n    $inner = function () use ($name) {\n        return \"${$name}\";\n    };\n    return $inner;\n}\n"
	doc := Collect(source)
	for _, s := range doc.Scopes {
		if want := s.Kind == ScopeClosure; s.AccessesVariablesByName != want {
			t.Errorf("%s scope %q reads its variables by name = %v, want %v", s.Kind, s.Name, s.AccessesVariablesByName, want)
		}
	}
}

func TestAnUnclosedDollarBraceAccessesVariablesByName(t *testing.T) {
	doc := Collect("<?php\n$label = 'x';\necho \"${")
	if !doc.Scopes[0].AccessesVariablesByName {
		t.Error("an unclosed ${ does not mark the file as reading its variables by name")
	}
}

func TestABracedNameWithoutAVariableReadsNothing(t *testing.T) {
	doc := Collect("<?php\nfunction run() {\n    return \"${missing}\";\n}\n")
	if len(doc.Bindings) != 0 {
		t.Errorf("bindings = %+v, want none", doc.Bindings)
	}
	if doc.Scopes[1].AccessesVariablesByName {
		t.Error("${missing} marked run() as reading its variables by name")
	}
}
