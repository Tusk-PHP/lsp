package checks

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func compileErrorMessages(rule *CompileErrorRule, src string) []string {
	var messages []string
	for _, f := range rule.Check(parser.ParseFile(src, token.PHP83), src, nil) {
		messages = append(messages, f.Message)
	}
	return messages
}

func TestCompileErrorsOnAViewMatchLaravelsCompiledView(t *testing.T) {
	templates, err := filepath.Glob("../framework/laravel/blade/testdata/controlflow/*.blade.php")
	if err != nil || len(templates) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, path := range templates {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".blade.php"), func(t *testing.T) {
			template, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			laravel, err := os.ReadFile(strings.TrimSuffix(path, ".blade.php") + ".compiled.php")
			if err != nil {
				t.Fatal(err)
			}
			got := compileErrorMessages(&CompileErrorRule{View: true}, blade.Compile(string(template)).PHP)
			want := compileErrorMessages(&CompileErrorRule{}, string(laravel))
			if !slices.Equal(got, want) {
				t.Errorf("got %q, Laravel's compiled view has %q", got, want)
			}
		})
	}
}
