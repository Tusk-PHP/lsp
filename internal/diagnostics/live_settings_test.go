package diagnostics

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
)

func TestBladeStructureFindingsFollowTheCurrentRules(t *testing.T) {
	const uri = "file:///tmp/resources/views/panel.blade.php"
	const view = "@if($open)\n<p>open</p>\n"
	p := newTestProvider()
	p.Blade = &blade.Features{}
	if got := filterByCode(p.Analyze(uri, view), "unclosed-directive"); len(got) != 1 {
		t.Fatalf("an @if without @endif is reported, got %+v", got)
	}
	cfg := *p.settings.Load().cfg
	if err := cfg.SetDiagnosticRulesJSON(`{"unclosed-directive": "off"}`); err != nil {
		t.Fatal(err)
	}
	p.SetConfig(&cfg)
	if got := filterByCode(p.Analyze(uri, view), "unclosed-directive"); len(got) != 0 {
		t.Errorf("a Blade rule the new configuration turns off stops reporting, got %+v", got)
	}
}

func writeTool(t *testing.T, dir, name, output string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\ncat <<'JSON'\n" + output + "\nJSON\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunToolsUsesTheToolsOfTheCurrentSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "Order.php")
	if err := os.WriteFile(file, []byte("<?php\nstrlen(1);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := "file://" + file
	phpstan := writeTool(t, dir, "phpstan", `{"totals": {"errors": 0, "file_errors": 1}, "files": {"`+file+`": {"errors": 1, "messages": [{"message": "Parameter #1 expects string", "line": 2, "identifier": "argument.type"}]}}}`)
	pint := writeTool(t, dir, "pint", `{"files": [{"name": "`+file+`", "appliedFixers": ["no_unused_imports"]}]}`)

	p := newTestProvider()
	p.RunTools(uri, file)
	if got := len(filterBySource(p.Analyze(uri, "<?php\nstrlen(1);\n"), "phpstan")); got != 0 {
		t.Fatalf("disabled tools report nothing, got %d", got)
	}

	cfg := *p.settings.Load().cfg
	enabled := true
	cfg.PHPStanEnabled, cfg.PHPStanPath = &enabled, phpstan
	cfg.PintEnabled, cfg.PintPath = &enabled, pint
	p.SetConfig(&cfg)
	p.RunTools(uri, file)
	diags := p.Analyze(uri, "<?php\nstrlen(1);\n")
	stan := filterBySource(diags, "phpstan")
	if len(stan) != 1 || stan[0].Code != "argument.type" || stan[0].Range.Start.Line != 1 {
		t.Errorf("PHPStan enabled by the new settings reports its finding, got %+v", stan)
	}
	if fixers := filterBySource(diags, "pint"); len(fixers) != 1 || fixers[0].Code != "no_unused_imports" {
		t.Errorf("Pint enabled by the new settings reports its fixer, got %+v", fixers)
	}
}
