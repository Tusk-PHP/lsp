package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// mapperChainSource mirrors open-southeners/laravel-data-mapper: a function
// returning Mapper, whose to() binds T from class-string<T> through a
// nullable native parameter with a default and no native return type.
const mapperChainSource = `<?php
namespace App;

function map(mixed $input): Mapper {}

class Mapper {
    /**
     * @template T of object
     *
     * @param  class-string<T>  $output
     * @return T
     */
    public function to(?string $output = null) {}
}

class CreateProjectData {
    public string $organization;
}

class Service {
    public function run(array $payload): void {
        $data = map($payload)->to(CreateProjectData::class);
        $plain = map($payload);
    }
}
`

// A function call followed by ->method() is typed by the whole chain, not by
// what the function alone returns.
func TestFunctionCallChainBindsMethodTemplate(t *testing.T) {
	r, file, lines := setupBoxResolver(t, mapperChainSource)
	r.TypedChainResolver = func(req *Resolver, expr, _ string, pos textpos.Position, f *parser.FileNode) ResolvedType {
		return req.ExpressionType(expr, ExprContext{Pos: pos, File: f})
	}

	pos := textpos.Position{Line: lineContaining(t, lines, "$data =")}
	if got := r.ResolveVariableTypeTyped("$data", lines, pos, file).FullString(); got != "App\\CreateProjectData" {
		t.Fatalf("$data = %q, want App\\CreateProjectData", got)
	}

	pos = textpos.Position{Line: lineContaining(t, lines, "$plain =")}
	if got := r.ResolveVariableTypeTyped("$plain", lines, pos, file).FullString(); got != "App\\Mapper" {
		t.Fatalf("$plain = %q, want App\\Mapper", got)
	}
}
