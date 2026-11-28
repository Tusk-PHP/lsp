package diagnostics

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
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

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }

func TestRunToolsDropsFindingsOfToolsTurnedOffDuringTheRun(t *testing.T) {
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
	pint := filepath.Join(dir, "pint")
	if err := os.WriteFile(pint, []byte("#!/bin/sh\necho broken >&2\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := newTestProvider()
	enabled := true
	cfg := *p.settings.Load().cfg
	cfg.PHPStanEnabled, cfg.PHPStanPath = &enabled, phpstan
	cfg.PintEnabled, cfg.PintPath = &enabled, pint
	off := cfg
	disabled := false
	off.PHPStanEnabled, off.PintEnabled = &disabled, &disabled
	armed, changed := false, false
	p.logger = log.New(writerFunc(func(b []byte) (int, error) {
		if armed && !changed {
			changed = true
			p.SetConfig(&off)
		}
		return len(b), nil
	}), "", 0)
	p.SetConfig(&cfg)
	armed = true
	p.RunTools(uri, file)
	if !changed {
		t.Fatal("the failing Pint run logs, which turns the tools off")
	}
	if stan := filterBySource(p.Analyze(uri, "<?php\nstrlen(1);\n"), "phpstan"); len(stan) != 0 {
		t.Errorf("a PHPStan finding of a run that outlived the settings turning PHPStan off is dropped, got %+v", stan)
	}
}

func TestAnalyzeOnSaveFollowsSettingsChangedDuringTheRun(t *testing.T) {
	const uri = "file:///t.php"
	p := builderProvider(t, `{}`)
	strict := *p.settings.Load().cfg
	if err := strict.SetDiagnosticRulesJSON(`{"unknown-relation": "error"}`); err != nil {
		t.Fatal(err)
	}
	resolve := p.BuilderModelResolver
	changed := false
	p.BuilderModelResolver = func(prefix, method, source string, line int, file *parser.FileNode) frameworklaravel.QueriedModel {
		if !changed {
			changed = true
			p.SetConfig(&strict)
		}
		return resolve(prefix, method, source, line, file)
	}
	p.AnalyzeOnSave(uri, builderSource)
	if !changed {
		t.Fatal("the on-save rules resolve the builder's model")
	}
	rels := filterByCode(p.Analyze(uri, builderSource), "unknown-relation")
	if len(rels) != 1 || rels[0].Severity != rule.SeverityError {
		t.Errorf("findings of a save follow settings that changed while it ran: %#v", rels)
	}
}

func TestReanalyzeSavedFollowsSettingsChangedDuringTheRun(t *testing.T) {
	const uri = "file:///t.php"
	p := builderProvider(t, `{}`)
	p.AnalyzeOnSave(uri, builderSource)
	errors := *p.settings.Load().cfg
	if err := errors.SetDiagnosticRulesJSON(`{"unknown-relation": "error"}`); err != nil {
		t.Fatal(err)
	}
	off := errors
	if err := off.SetDiagnosticRulesJSON(`{"unknown-relation": "off"}`); err != nil {
		t.Fatal(err)
	}
	resolve := p.BuilderModelResolver
	changed := false
	p.BuilderModelResolver = func(prefix, method, source string, line int, file *parser.FileNode) frameworklaravel.QueriedModel {
		if !changed {
			changed = true
			p.SetConfig(&off)
		}
		return resolve(prefix, method, source, line, file)
	}
	p.SetConfig(&errors)
	p.ReanalyzeSaved()
	if !changed {
		t.Fatal("the reanalysis resolves the builder's model")
	}
	if rels := filterByCode(p.Analyze(uri, builderSource), "unknown-relation"); len(rels) != 0 {
		t.Errorf("a reanalysis follows settings that changed while it ran: %#v", rels)
	}
}
