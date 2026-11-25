package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func TestFunctionNames(t *testing.T) {
	src := `<?php
namespace App;

use Vendor\Lib;
use function Other\helper;
`
	file := parser.ParseFile(src)
	cases := map[string]string{
		`\Uri\WhatWg\url_percent_encode`: `Uri\WhatWg\url_percent_encode`,
		`Lib\run`:                        `Vendor\Lib\run`,
		`Sub\run`:                        `App\Sub\run`,
		`helper`:                         `Other\helper`,
		`strlen`:                         `App\strlen Global fallback`,
	}
	for name, want := range cases {
		got, ok := FunctionNames(name, file)
		if !ok {
			t.Fatalf("%s: not resolved", name)
		}
		if name == "strlen" {
			if strings.Join(got, " ") != `App\strlen strlen` {
				t.Errorf("%s: got %v, want the namespaced name then the global one", name, got)
			}
			continue
		}
		if strings.Join(got, " ") != want {
			t.Errorf("%s: got %v, want %s", name, got, want)
		}
	}
}
