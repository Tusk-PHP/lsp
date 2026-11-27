package sourcectx_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func cursorFeatureBenchSource() (string, protocol.Position) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\n\nuse App\\Models\\User;\n\nclass Room\n{\n")
	line := 7
	var at protocol.Position
	for i := range 300 {
		src.WriteString("    public function run" + strconv.Itoa(i) + "(User $user): string\n    {\n")
		src.WriteString("        $fn = fn ($x) => $x . $user->name;\n")
		if i == 150 {
			at = protocol.Position{Line: line + 3, Character: len("        return $user->rena")}
		}
		src.WriteString("        return $user->rename($fn('a'))->name;\n    }\n\n")
		line += 6
	}
	src.WriteString("}\n")
	return src.String(), at
}

func BenchmarkCursorFeature(b *testing.B) {
	source, pos := cursorFeatureBenchSource()
	f := newFeatures(source)
	read := func(string) string { return "" }
	hints := &config.InlayHintsConfig{Enabled: true, VariableTypes: true, ForeachTypes: true, ClosureReturnTypes: true, ReturnTypes: true, ParameterNames: true}
	for _, feature := range []struct {
		name string
		run  func()
	}{
		{"Hover", func() { f.hover.GetHover(featureURI, source, pos) }},
		{"HoverVariable", func() {
			f.hover.GetHover(featureURI, source, protocol.Position{Line: pos.Line, Character: len("        return $us")})
		}},
		{"Completion", func() { f.comp.GetCompletions(featureURI, source, pos) }},
		{"Definition", func() { f.a.FindDefinition(featureURI, source, pos) }},
		{"References", func() { f.a.FindAllReferences(featureURI, source, pos, read) }},
		{"Highlights", func() { f.a.GetDocumentHighlights(featureURI, source, pos) }},
		{"DocumentSymbols", func() { f.a.GetDocumentSymbols(featureURI, source) }},
		{"InlayHints", func() { f.hints.GetInlayHints(featureURI, source, hints) }},
		{"CodeActions", func() {
			f.a.GetCodeActions(featureURI, source, protocol.CodeActionParams{TextDocument: protocol.TextDocumentIdentifier{URI: featureURI}, Range: protocol.Range{Start: pos, End: pos}})
		}},
		{"FileNamespace", func() { f.a.GetFileNamespace(featureURI, source) }},
		{"MoveToNamespace", func() { f.a.MoveToNamespace(featureURI, source, "App\\Moved", nil, read) }},
	} {
		b.Run(feature.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				feature.run()
			}
		})
	}
}
