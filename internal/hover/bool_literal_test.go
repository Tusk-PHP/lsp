package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverShowsTrueAndFalseInDeclaredTypes(t *testing.T) {
	src := `<?php
/** @return ?true */
function confirmed() {}
/** @return true|false */
function either() {}
/** @phpstan-assert-if-true false $value */
function failed($value): bool {}
/** @phpstan-assert !false $value */
function succeeded($value): void {}
/** @param string|false $read */
function take($read) {
    if (failed($read)) {
        echo $read;
    }
    succeeded($read);
    return $read;
}
$contents = file_get_contents('a.txt');
$contents;
$flag = confirmed();
$flag;
$any = either();
$any;
`
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///bool.php", src)
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	at := func(needle string) protocol.Position {
		i := strings.Index(src, needle)
		if i < 0 {
			t.Fatalf("%q not in source", needle)
		}
		i += strings.Index(needle, "$")
		return protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n")}
	}
	for needle, want := range map[string]string{
		"$contents;":    "string|false $contents",
		"$flag;":        "?true $flag",
		"$any;":         "bool $any",
		"echo $read;":   "false $read",
		"return $read;": "string $read",
	} {
		h := p.GetHover("file:///bool.php", src, at(needle))
		if h == nil || !strings.Contains(h.Contents.Value, "\n"+want+"\n") {
			t.Errorf("hover on %q: want %q, got %+v", needle, want, h)
		}
	}
}
