package diagnostics

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// TestAnalyzeBladeChecksOnlyItsPHP checks that a Blade view is checked through
// the PHP it contains: directives, components, HTML and the variables a view
// receives aren't reported, and a real mistake inside `{{ }}` is reported at
// its place in the view.
func TestAnalyzeBladeChecksOnlyItsPHP(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///app/User.php", "<?php\nnamespace App;\nclass User { public function name(): string {} }\n")
	idx.MarkReady()
	p := NewProvider(idx, "laravel", "/tmp", nil, config.DefaultConfig())

	view := `@extends('layouts.app')
@section('content')
    @props(['title' => 'Hi'])
    <x-mail::message :url="$url">
        {{ $slot }} {{ $attributes->get('class') }}
    </x-mail::message>
    @foreach($users as $user)
        <p @click="open = true">{{ $loop->index }}: {{ strtoupper($user->name) }}</p>
        @include('partials.row', ['user' => $user])
    @endforeach
    {{ missing_helper($title) }}
@endsection
`
	diags := p.Analyze("file:///resources/views/users.blade.php", view)
	if len(diags) != 1 {
		for _, d := range diags {
			t.Logf("%v %s", d.Range, d.Message)
		}
		t.Fatalf("got %d diagnostics, want only the unknown function", len(diags))
	}
	d := diags[0]
	line := strings.Split(view, "\n")[d.Range.Start.Line]
	if !strings.Contains(d.Message, "missing_helper") || line[d.Range.Start.Character:d.Range.End.Character] != "missing_helper" {
		t.Errorf("got %q at %v (%q)", d.Message, d.Range, line)
	}
}

// TestAnalyzeBladeFacadeAliases checks that Laravel's facade aliases, used in
// a view without an import, aren't unknown classes, while a class that is no
// alias still is.
func TestAnalyzeBladeFacadeAliases(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///vendor/Route.php", "<?php\nnamespace Illuminate\\Support\\Facades;\n/** @method static bool has(string $name) */\nclass Route {}\n")
	idx.IndexFile("file:///vendor/helpers.php", "<?php\nfunction route($name) {}\n")
	aliases := frameworklaravel.NewAliases(t.TempDir(), func() phpparser.GrammarRange { return phpparser.OnlyGrammar(token.Latest) })
	aliases.Load()
	idx.SetClassGenerator(aliases)
	idx.MarkReady()
	p := NewProvider(idx, "laravel", "/tmp", nil, config.DefaultConfig())

	view := `@if (Route::has('login'))
    <a href="{{ route('login') }}">Log in</a>
@endif
{{ Nope::make() }}
`
	diags := p.Analyze("file:///resources/views/welcome.blade.php", view)
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "'Nope'") {
		for _, d := range diags {
			t.Logf("%v %s", d.Range, d.Message)
		}
		t.Fatalf("got %d diagnostics, want only the unknown class Nope", len(diags))
	}
}
