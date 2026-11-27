package container

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestCallTypes(t *testing.T) {
	idx := symbols.NewIndex()
	src := "<?php\nnamespace App;\nuse Illuminate\\Http\\Request;\nclass Report {}\n"
	idx.IndexFile("file:///app/Report.php", src)
	ca := NewContainerAnalyzer(idx, "/tmp", "laravel")
	ca.Analyze()
	request := ca.ResolveDependency("request")
	config := ca.ResolveDependency("config")
	if request == nil || config == nil {
		t.Fatal("the Laravel defaults have no request or config binding")
	}
	callType := CallTypes(ca, idx)
	r := resolve.NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
	for expr, want := range map[string]string{
		"app('request')":       request.Concrete,
		"resolve('request')":   request.Concrete,
		"config()":             config.Concrete,
		"config('app.name')":   "-",
		"app(Report::class)":   `App\Report`,
		"app('App\\Report')":   `App\Report`,
		"app('nothing.bound')": "",
		"strlen('x')":          "",
	} {
		if got := callType(r, expr, src, file); got != want {
			t.Errorf("CallTypes(%q) = %q, want %q", expr, got, want)
		}
	}
}

// Without a config binding, config() is still the Repository.
func TestCallTypesConfigWithoutBinding(t *testing.T) {
	idx := symbols.NewIndex()
	ca := NewContainerAnalyzer(idx, "/tmp", "none")
	ca.Analyze()
	if got := CallTypes(ca, idx)(resolve.NewResolver(idx), "config()", "", nil); got != `Illuminate\Config\Repository` {
		t.Errorf("config() = %q", got)
	}
}
