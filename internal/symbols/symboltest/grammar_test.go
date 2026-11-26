package symboltest

import (
	"fmt"
	"strings"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type fatalRecorder struct {
	testing.TB
	message string
}

type stopped struct{}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.message = fmt.Sprintf(format, args...)
	panic(stopped{})
}

func pinFailure(t *testing.T, idx *symbols.Index, source string) (message string) {
	t.Helper()
	recorder := &fatalRecorder{TB: t}
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(stopped); !ok {
				panic(r)
			}
		}
		message = recorder.message
	}()
	PinToPHP74(recorder, idx, "file:///pinned.php", source)
	return ""
}

func TestPinToPHP74(t *testing.T) {
	const differs = "<?php\nnamespace App;\n\n#[A] function f() {} use B\\C;\n"
	wide := func() *symbols.Index {
		return symbols.NewIndexFor(phpparser.GrammarsBetween(token.PHP74, token.Latest))
	}
	tests := []struct {
		name   string
		idx    *symbols.Index
		source string
		want   string
	}{
		{"pins a file both read cleanly", wide(), differs, ""},
		{"needs PHP 7.4 in the range", symbols.NewIndexFor(phpparser.OnlyGrammar(token.Latest)), differs, "want a range from PHP 7.4"},
		{"needs a text PHP 7.4 reads cleanly", wide(), "<?php\nenum Suit {}\n", "want a clean PHP 7.4 tree"},
		{"needs a text the newest grammar reads cleanly", wide(), "<?php\nfunction match() {}\n", "want a clean tree of the newest grammar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pinFailure(t, tc.idx, tc.source)
			if tc.want != "" {
				if !strings.Contains(got, tc.want) {
					t.Errorf("PinToPHP74 failed with %q, want %q", got, tc.want)
				}
				return
			}
			if got != "" {
				t.Fatalf("PinToPHP74 failed with %q", got)
			}
			if tree := tc.idx.GrammarOf("file:///pinned.php").Parse(tc.source); tree.Version != token.PHP74 {
				t.Errorf("the index reads the file as PHP %s, want PHP 7.4", tree.Version)
			}
		})
	}
}
