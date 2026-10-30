package hover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/coverage"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const coverageCardSource = `<?php
namespace App;

class Service {
    public function handle(): void {
        echo 1;
        echo 2;
    }

    public function missing(): void {}
}
`

const coverageCardReport = `<?xml version="1.0"?>
<coverage generated="1"><project timestamp="1">
  <file name="/var/www/html/app/Service.php">
    <line num="5" type="method" name="handle" complexity="2" crap="4.5" count="1"/>
    <line num="6" type="stmt" count="1"/>
    <line num="7" type="stmt" count="0"/>
  </file>
</project></coverage>
`

func coverageCard(t *testing.T, report string, sourceAge time.Duration, word string) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "app", "Service.php")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(coverageCardSource), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(source, now.Add(sourceAge), now.Add(sourceAge)); err != nil {
		t.Fatal(err)
	}
	if report != "" {
		if err := os.WriteFile(filepath.Join(root, "clover.xml"), []byte(report), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	uri := "file://" + source
	idx := symbols.NewIndex()
	idx.IndexFile(uri, coverageCardSource)
	p := NewProvider(idx, nil, "none")
	store := coverage.NewStore(root, "", nil)
	store.Refresh()
	p.SetCoverage(store)

	for i, l := range strings.Split(coverageCardSource, "\n") {
		if strings.Contains(l, "function "+word) {
			h := p.GetHover(uri, coverageCardSource, protocol.Position{Line: i, Character: strings.Index(l, word) + 1})
			if h == nil {
				t.Fatalf("expected hover on %q", word)
			}
			return h.Contents.Value
		}
	}
	t.Fatalf("no declaration of %q", word)
	return ""
}

func TestHoverShowsMethodCoverage(t *testing.T) {
	val := coverageCard(t, coverageCardReport, -time.Hour, "handle")
	if !strings.Contains(val, "Coverage: 🟡 `█████░░░░░` 50% · 1/2 lines · CRAP 4.5\n") {
		t.Fatalf("expected a coverage line, got:\n%s", val)
	}
}

func TestHoverMarksStaleCoverage(t *testing.T) {
	val := coverageCard(t, coverageCardReport, time.Hour, "handle")
	if !strings.Contains(val, "may be out of date") {
		t.Fatalf("expected a stale note, got:\n%s", val)
	}
}

func TestHoverOmitsUnknownCoverage(t *testing.T) {
	for name, val := range map[string]string{
		"no report":     coverageCard(t, "", -time.Hour, "handle"),
		"not in report": coverageCard(t, coverageCardReport, -time.Hour, "missing"),
	} {
		if strings.Contains(val, "Coverage") {
			t.Errorf("%s: expected no coverage line, got:\n%s", name, val)
		}
	}
}

func TestCoverageBar(t *testing.T) {
	cases := []struct {
		covered, total int
		want           string
	}{
		{0, 9, "🔴 `░░░░░░░░░░` 0%"},
		{2, 9, "🔴 `██░░░░░░░░` 22%"},
		{1, 1000, "🔴 `█░░░░░░░░░` 1%"},
		{1, 2, "🟡 `█████░░░░░` 50%"},
		{9, 10, "🟢 `█████████░` 90%"},
		{999, 1000, "🟢 `█████████░` 99%"},
		{3, 3, "🟢 `██████████` 100%"},
	}
	for _, c := range cases {
		if got := coverageBar(c.covered, c.total); got != c.want {
			t.Errorf("coverageBar(%d, %d) = %q, want %q", c.covered, c.total, got, c.want)
		}
	}
}
