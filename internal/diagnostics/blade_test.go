package diagnostics

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
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
