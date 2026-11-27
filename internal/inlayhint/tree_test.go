package inlayhint

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

const treeSource = `<?php
namespace App;

class Other {
    public function name(): string { return ''; }
}

function twice(int $first, int $second): int { return $first * $second; }

function spread(int $head, int ...$rest): int { return $head; }

trait Named {
    /** @return Other */
    public function other() { return new Other(); }

    /** A doc without a return type. */
    public function plain() {}
}

class K {
    public static function pair(int $left, int $right): int { return $left + $right; }

    public function make() {
        $arrow = fn () => $this->named();
        $o = new Other();
        $plain = function () use ($o) { return $o; };
        $sum = static::pair(1, 2);
        static::$cache = $sum;
        twice(1, $assigned = new Other());
        $m = 'name';
        $o->$m(1, 2);
        undefined_function(1, 2);
        $unknown->pair(1, 2);
        $o->missing(1, 2);
        $called = strlen(...);
        $set = isset($a, $b);
        $path = '/' . trim('x');
        $trimmed = trim('x') . '/';
        foreach ([$o] as $i => &$ref) {}
        foreach ([[$o]] as [$first]) {}
        static $count = new Other();
        $attributed = #[Attr(1)] fn (int $n) => new Other();
        $paren = ('x');
        spread(1, 2, 3);
        return $arrow;
    }

    public function named(): Other { return new Other(); }
}
`

func treeHints(t *testing.T, source string) map[string][]string {
	t.Helper()
	p := setupProvider(t, "file:///tree.php", source)
	cfg := allCfg()
	cfg.SuppressSingleParam = false
	cfg.SuppressNameMatch = false
	got := map[string][]string{}
	for _, h := range p.GetInlayHints("file:///tree.php", source, cfg) {
		key := positionKey(h.Position)
		got[key] = append(got[key], h.Label)
	}
	return got
}

func positionKey(p protocol.Position) string {
	return strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Character)
}

func after(t *testing.T, source, marker string) string {
	t.Helper()
	i := strings.Index(source, marker)
	if i < 0 {
		t.Fatalf("marker %q not in source", marker)
	}
	end := i + len(marker)
	line := strings.Count(source[:end], "\n")
	return positionKey(protocol.Position{Line: line, Character: end - strings.LastIndexByte(source[:end], '\n') - 1})
}

func lineOfMarker(t *testing.T, source, marker string) int {
	t.Helper()
	i := strings.Index(source, marker)
	if i < 0 {
		t.Fatalf("marker %q not in source", marker)
	}
	return strings.Count(source[:i], "\n")
}

func TestHintsAreReadFromTheSyntaxTree(t *testing.T) {
	got := treeHints(t, treeSource)
	want := map[string]string{
		"fn () => $this->named()": ": Other",
		"$plain = function ()":    ": Other",
		"static::pair(":           "left:",
		"static::pair(1, ":        "right:",
		"twice(1, ":               "second:",
		"$assigned":               ": Other",
		"public function other()": ": Other",
		"foreach ([$o] as $i":     ": int",
		"$trimmed":                ": string",
		"#[Attr(1)] fn (int $n)":  ": Other",
		"        spread(":         "head:",
	}
	for marker, label := range want {
		key := after(t, treeSource, strings.Split(marker, " => ")[0])
		if !slices.Contains(got[key], label) {
			t.Errorf("after %q: want %q, got %v", marker, label, got[key])
		}
	}
	for _, marker := range []string{
		"fn () => $this->named()",
		"static::$cache",
		"$path",
		"&$ref",
		"[$first]",
		"$paren",
		"public function plain()",
		"spread(1, ",
	} {
		if labels := got[after(t, treeSource, marker)]; len(labels) > 0 {
			t.Errorf("after %q: want no hint, got %v", marker, labels)
		}
	}
	for _, marker := range []string{
		"function twice(",
		"$o->$m(",
		"undefined_function(",
		"$unknown->pair(",
		"$o->missing(",
		"$called = strlen(",
		"$set = isset(",
	} {
		line := lineOfMarker(t, treeSource, marker)
		for key, labels := range got {
			for _, l := range labels {
				if strings.HasPrefix(key, strconv.Itoa(line)+":") && strings.HasSuffix(l, ":") {
					t.Errorf("line of %q: want no parameter hint, got %q at %s", marker, l, key)
				}
			}
		}
	}
	if labels := got[after(t, treeSource, "static $count")]; len(labels) != 1 {
		t.Errorf("static variable: want one type hint, got %v", labels)
	}
}

func TestHintsOnIncompleteCode(t *testing.T) {
	for _, source := range []string{
		"<?php\nforeach ($a as $b)",
		"<?php\nforeach ($a as $b) ?>",
		"<?php\nfunction () { return $nothing; };",
		"<?php\nfunction () use ($x)",
		"<?php\nfn => new \\DateTime();",
		"<?php\nclass A {\n    /** @return \\DateTime */\n    public function a\n}\n",
	} {
		for _, h := range setupProvider(t).GetInlayHints("file:///broken.php", source, allCfg()) {
			t.Errorf("%q: want no hints, got %q at %+v", source, h.Label, h.Position)
		}
	}
}
