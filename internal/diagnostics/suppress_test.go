package diagnostics

import (
	"encoding/json"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func codesAt(diags []protocol.Diagnostic) map[int][]string {
	out := map[int][]string{}
	for _, d := range diags {
		out[d.Range.Start.Line] = append(out[d.Range.Start.Line], d.Source+":"+d.Code)
	}
	return out
}

func TestInlineSuppressionDropsNativeFindings(t *testing.T) {
	p := newTestProvider()
	p.index.MarkReady()
	source := `<?php
function run() {
    // @tusk-ignore undefined-variable
    echo $first;
    echo $second; // @tusk-ignore unknown-function
    echo $third; // @tusk-ignore
    echo $fourth;
}
`
	got := codesAt(p.Analyze("file:///tmp/run.php", source))
	if _, ok := got[3]; ok {
		t.Errorf("line 3 is suppressed by the comment above it: %v", got[3])
	}
	if len(got[4]) != 1 || got[4][0] != "tusk-php:undefined-variable" {
		t.Errorf("line 4 suppresses another code, so undefined-variable stays: %v", got[4])
	}
	if _, ok := got[5]; ok {
		t.Errorf("line 5 has a bare suppression: %v", got[5])
	}
	if len(got[6]) != 1 {
		t.Errorf("line 6 is not suppressed: %v", got[6])
	}
}

func TestInlineSuppressionDropsToolFindings(t *testing.T) {
	p := newTestProvider()
	p.index.MarkReady()
	uri := "file:///tmp/tool.php"
	source := "<?php\n// @tusk-ignore phpstan:argument.type\nstrlen(1);\nstrlen(2); // @tusk-ignore phpstan\nstrlen(3);\n"
	p.toolResults[uri] = []protocol.Diagnostic{
		{Range: textpos.Range{Start: textpos.Position{Line: 2}}, Source: "phpstan", Code: "argument.type", Message: "a"},
		{Range: textpos.Range{Start: textpos.Position{Line: 3}}, Source: "phpstan", Code: "argument.type", Message: "b"},
		{Range: textpos.Range{Start: textpos.Position{Line: 4}}, Source: "phpstan", Code: "argument.type", Message: "c"},
	}
	diags := p.Analyze(uri, source)
	if len(diags) != 1 || diags[0].Message != "c" {
		t.Errorf("only the unsuppressed PHPStan finding stays, got %+v", diags)
	}
}

func TestIgnoreRulesDropFindingsByPathCodeAndMessage(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.MarkReady()
	cfg := config.DefaultConfig()
	if err := json.Unmarshal([]byte(`[
		{"path": "app/Legacy/**"},
		{"code": "undefined-variable", "message": "\\$generated"}
	]`), &cfg.DiagnosticIgnore); err != nil {
		t.Fatal(err)
	}
	p := NewProvider(idx, "none", "/project", nil, cfg)
	source := "<?php\nfunction f() {\n    echo $generated;\n    echo $typo;\n}\n"

	if diags := p.Analyze("file:///project/app/Legacy/Old.php", source); len(diags) != 0 {
		t.Errorf("everything under app/Legacy is ignored, got %+v", diags)
	}
	got := codesAt(p.Analyze("file:///project/app/New.php", source))
	if _, ok := got[2]; ok {
		t.Errorf("$generated is ignored by message: %v", got[2])
	}
	if len(got[3]) != 1 {
		t.Errorf("$typo is still reported: %v", got)
	}
	if diags := p.Analyze("file:///elsewhere/app/Legacy/Old.php", source); len(diags) == 0 {
		t.Error("a path rule only applies inside the project root")
	}
}

func TestInlineSuppressionInBladeView(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.MarkReady()
	p := NewProvider(idx, "laravel", "/tmp", nil, config.DefaultConfig())
	view := "{{-- @tusk-ignore unknown-function --}}\n{{ missing_one() }}\n{{ missing_two() }} {{-- @tusk-ignore --}}\n{{ missing_three() }}\n"
	got := codesAt(p.Analyze("file:///tmp/resources/views/x.blade.php", view))
	if len(got) != 1 || len(got[3]) != 1 {
		t.Errorf("only missing_three is reported, got %v", got)
	}
}

func TestExistingPHPStanIgnoresSilenceTuskFindings(t *testing.T) {
	p := newTestProvider()
	p.index.MarkReady()
	uri := "file:///tmp/phpstan.php"
	source := `<?php
function run() {
    // @phpstan-ignore variable.undefined
    echo $first;
    echo $second; // @phpstan-ignore argument.type
    // @phpstan-ignore-next-line
    echo $third;
    echo $fourth; // @phpstan-ignore-line
    echo $fifth;
}
`
	p.toolResults[uri] = []protocol.Diagnostic{
		{Range: textpos.Range{Start: textpos.Position{Line: 4}}, Source: "phpstan", Code: "argument.type", Message: "ignored by PHPStan's own comment"},
	}
	got := codesAt(p.Analyze(uri, source))
	for _, line := range []int{3, 6, 7} {
		if _, ok := got[line]; ok {
			t.Errorf("line %d is silenced by a @phpstan-ignore comment: %v", line, got[line])
		}
	}
	if len(got[4]) != 1 || got[4][0] != "tusk-php:undefined-variable" {
		t.Errorf("argument.type silences PHPStan's finding but not Tusk's undefined-variable: %v", got[4])
	}
	if len(got[8]) != 1 {
		t.Errorf("line 8 has no comment: %v", got[8])
	}
}

func TestPHPStanIgnoreSilencesUnusedPrivateConstant(t *testing.T) {
	p := newTestProvider()
	p.index.MarkReady()
	source := `<?php
class Settings {
    // @phpstan-ignore classConstant.unused
    private const KEPT = 1;
    private const DROPPED = 2;
}
`
	got := codesAt(p.Analyze("file:///tmp/settings.php", source))
	if _, ok := got[3]; ok {
		t.Errorf("classConstant.unused silences unused-private-constant: %v", got[3])
	}
	if len(got[4]) != 1 || got[4][0] != "tusk-php:unused-private-constant" {
		t.Errorf("the constant without a comment is still reported: %v", got[4])
	}
}

func TestIgnoreRulesWithoutPathApplyOutsideTheRoot(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.MarkReady()
	cfg := config.DefaultConfig()
	if err := json.Unmarshal([]byte(`[{"code": "undefined-variable"}, {"path": "**"}]`), &cfg.DiagnosticIgnore); err != nil {
		t.Fatal(err)
	}
	p := NewProvider(idx, "none", "relative/root", nil, cfg)
	diags := p.Analyze("file:///abs/run.php", "<?php\nfunction f() {\n    echo $x;\n    missing();\n}\n")
	if len(diags) != 1 || diags[0].Code != "unknown-function" {
		t.Errorf("the code rule applies without a root, the path rule doesn't: %+v", diags)
	}
}
