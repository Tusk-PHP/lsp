package blade

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func kinds(nodes []Node, src string) []string {
	var out []string
	for _, n := range nodes {
		switch n.Kind {
		case KindEcho:
			out = append(out, "echo:"+src[n.Expr.Start:n.Expr.End])
		case KindDirective:
			s := "@" + n.Name
			if n.HasArgs {
				s += "(" + src[n.Args.Start:n.Args.End] + ")"
			}
			out = append(out, s)
		case KindPHP:
			out = append(out, "php:"+strings.TrimSpace(src[n.Expr.Start:n.Expr.End]))
		case KindBoundAttribute:
			out = append(out, "bound:"+src[n.Expr.Start:n.Expr.End])
		case KindComponent:
			out = append(out, "<"+n.Name+">")
		}
	}
	return out
}

func TestParse(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"echoes", `<p>{{ $user->name }} {!! $html !!}</p>`, "echo: $user->name |echo: $html "},
		{"comment", `{{-- {{ $hidden }} @if($x) --}}{{ $shown }}`, "echo: $shown "},
		{"escapes", `@@if @{{ $raw }} {{ $real }}`, "echo: $real "},
		{"email and alpine", `mail@example.com <button @click="open = true">`, ""},
		{"directive args", "@include('partials.nav', ['user' => $user])\n@csrf", "@include('partials.nav', ['user' => $user])|@csrf"},
		{"args with parens in strings", `@if(str_contains($a, ')')) ok @endif`, "@if(str_contains($a, ')'))|@endif"},
		{"space before args", "@foreach ($items as $item)\n@endforeach", "@foreach($items as $item)|@endforeach"},
		{"php block", "@php\n    $total = 0;\n@endphp", "php:$total = 0;"},
		{"php inline", "@php($n = 1)", "php:$n = 1"},
		{"raw php", "<?php echo $a; ?>", "php:echo $a;"},
		{"verbatim", "@verbatim {{ $js }} @if @endverbatim{{ $b }}", "echo: $b "},
		{"component", `<x-alert type="error" :message="$msg" :$user ::class="open" class="{{ $c }}" wire:model="x" />`,
			"<alert>|bound:$msg|bound:$user|echo: $c "},
		{"component echo with ternary", `<x-a class="{{ $a ? 'x' : 'y' }}" :b="$b">`, "<a>|echo: $a ? 'x' : 'y' |bound:$b"},
		{"livewire", `<livewire:counter :count="$n" />`, "<livewire:counter>|bound:$n"},
		{"html bound attrs are alpine", `<div :class="{ open: isOpen }">`, ""},
		{"unknown directive", `@datetime($x) @click`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(kinds(Parse(tt.src), tt.src), "|")
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestCompileIsValidPHP(t *testing.T) {
	src := `@extends('layouts.app')
@use('App\Models\User')
@section('content')
    @if($users->isEmpty())
        <p>None</p>
    @elseif($users->count() > 10)
        <p>Many</p>
    @else
        @foreach($users as $user)
            @continue($user->hidden)
            <x-user-card :user="$user" />
        @endforeach
    @endif
    @forelse($posts as $post)
        {{ $post->title }}
    @empty
        <p>No posts</p>
    @endforelse
    @switch($type)
        @case(1)
            One
            @break
        @default
            Other
    @endswitch
    @auth <a>{{ auth()->user()->name }}</a> @endauth
    @unless($hidden) shown @endunless
    @isset($title) {{ $title }} @endisset
    @php $n = count($users); @endphp
@endsection
`
	doc := Compile(src)
	if !doc.Structured {
		t.Fatal("balanced template compiled flat")
	}
	file := parser.ParseFile(doc.PHP)
	if len(file.Errors) > 0 {
		t.Fatalf("virtual PHP has syntax errors %v:\n%s", file.Errors, doc.PHP)
	}
	if !strings.Contains(doc.PHP, "use App\\Models\\User;") {
		t.Errorf("@use not imported:\n%s", doc.PHP)
	}
}

func TestCompileUnbalancedIsFlat(t *testing.T) {
	doc := Compile("@if($a)\n{{ $b }}\n@foreach($c as $d)\n@endif")
	if doc.Structured {
		t.Fatal("unbalanced template should compile flat")
	}
	if file := parser.ParseFile(doc.PHP); len(file.Errors) > 0 {
		t.Fatalf("flat PHP has syntax errors %v:\n%s", file.Errors, doc.PHP)
	}
}

func TestPositionsMapBack(t *testing.T) {
	src := "<div>\n  <p>{{ $user->name }}</p>\n  @if($user->isAdmin())\n@endif\n</div>"
	doc := Compile(src)
	for _, want := range []string{"$user->name", "$user->isAdmin()"} {
		phpOff := strings.Index(doc.PHP, want)
		tplOff, ok := doc.TemplateOffset(phpOff)
		if !ok || !strings.HasPrefix(src[tplOff:], want) {
			t.Errorf("%s: maps to %d (%v)", want, tplOff, ok)
		}
	}
	// `echo (` is generated, so it maps nowhere.
	if _, ok := doc.TemplateOffset(strings.Index(doc.PHP, "echo")); ok {
		t.Error("generated code mapped to the template")
	}
	// Line and column: `isAdmin` is on line 2, column 13.
	php := strings.Index(doc.PHP, "isAdmin")
	line := strings.Count(doc.PHP[:php], "\n")
	col := php - strings.LastIndex(doc.PHP[:php], "\n") - 1
	if l, c, ok := doc.TemplatePosition(line, col); !ok || l != 2 || c != 13 {
		t.Errorf("isAdmin maps to %d:%d (%v), want 2:13", l, c, ok)
	}
	// A range ending in generated code is cut to the copied text.
	sl, sc, el, ec, ok := doc.TemplateRange(line, col, line, col+100)
	if !ok || sl != 2 || sc != 13 || el != 2 || ec != 22 {
		t.Errorf("range maps to %d:%d-%d:%d (%v), want 2:13-2:22", sl, sc, el, ec, ok)
	}
}

// BenchmarkCompile compiles a 1,000-line view, the work added to each change of
// a Blade file before its PHP is checked.
func BenchmarkCompile(b *testing.B) {
	var src strings.Builder
	for i := 0; i < 100; i++ {
		src.WriteString(`<div class="card {{ $active ? 'on' : '' }}">
    @if($user->isAdmin())
        <x-badge :label="$user->role" type="admin" />
    @endif
    @foreach($posts as $post)
        <a href="{{ route('posts.show', $post) }}" @click="open = true">{{ $post->title }}</a>
    @endforeach
    {{-- a comment --}}
    @include('partials.footer', ['year' => now()->year])
</div>
`)
	}
	tpl := src.String()
	b.SetBytes(int64(len(tpl)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Compile(tpl)
	}
}

func TestPHPPositionRoundTrip(t *testing.T) {
	tpl := "<p>\n  {{ $user->name }}\n</p>\n@if($a)\n"
	doc := Compile(tpl)
	// On `name`, in the echo.
	line, col, ok := doc.PHPPosition(1, 13)
	if !ok {
		t.Fatal("expected the echo to map into the PHP")
	}
	if l, c, ok := doc.TemplatePosition(line, col); !ok || l != 1 || c != 13 {
		t.Fatalf("round trip gave %d:%d (%v)", l, c, ok)
	}
	// Just after the expression, where a cursor sits while typing.
	if _, _, ok := doc.PHPPosition(1, 17); !ok {
		t.Fatal("expected the end of the expression to map")
	}
	// HTML and the directive's name are not PHP.
	if _, _, ok := doc.PHPPosition(0, 1); ok {
		t.Fatal("HTML mapped into the PHP")
	}
	if _, _, ok := doc.PHPPosition(3, 1); ok {
		t.Fatal("a directive's name mapped into the PHP")
	}
	if _, _, ok := doc.PHPPosition(3, 4); !ok {
		t.Fatal("expected the directive's argument to map")
	}
}

func TestCommentsListsBladeComments(t *testing.T) {
	src := "<p>{{-- one --}}</p>\n@if($a)\n{{-- two\nlines --}}\n@endif\n{{-- unclosed"
	got := Comments(src)
	want := []string{"{{-- one --}}", "{{-- two\nlines --}}", "{{-- unclosed"}
	if len(got) != len(want) {
		t.Fatalf("got %d comments, want %d", len(got), len(want))
	}
	for i, span := range got {
		if src[span.Start:span.End] != want[i] {
			t.Errorf("comment %d: got %q, want %q", i, src[span.Start:span.End], want[i])
		}
	}
}
