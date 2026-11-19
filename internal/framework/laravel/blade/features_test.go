package blade

import (
	"sort"
	"strings"
	"testing"
)

func TestCheckStructure(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string // messages, in order
	}{
		{"balanced", "@if($a) x @elseif($b) y @else z @endif @foreach($xs as $x) @endforeach", nil},
		{"conditional closed by another conditional's closer", "@auth x @endif", nil},
		{"forelse with empty", "@forelse($xs as $x) a @empty b @endforelse", nil},
		{"inline section", "@section('title', 'Home') @yield('x')", nil},
		{"block section closed by show", "@section('sidebar') x @show", nil},
		{"inline push and slot", "@push('scripts', '<script></script>') @slot('title', 'Hi')", nil},
		{"slot block with attributes", "@slot('title', null, ['class' => 'a']) x @endslot", nil},
		{"unclosed if", "@if($a) x", []string{"`@if` is never closed with `@endif`"}},
		{"stray end", "x @endforeach", []string{"`@endforeach` has no opening directive to close"}},
		{"mismatched", "@foreach($xs as $x) @if($x) @endforeach", []string{"`@if` is not closed: `@endforeach` closes an outer block first"}},
		{"else outside if", "@foreach($xs as $x) @else @endforeach", []string{"`@else` is outside an `@if` block"}},
		{"custom if keeps else quiet", "@admin a @else b @endadmin", nil},
		{"verbatim is skipped", "@verbatim @if @endverbatim", nil},
		{"comments are skipped", "{{-- @if($a) --}}", nil},
		{"email is text", "mail me@if.com", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, p := range CheckStructure(c.src, Parse(c.src)) {
				got = append(got, p.Message)
				if !strings.HasPrefix(c.src[p.Span.Start:p.Span.End], "@") {
					t.Errorf("problem span %q doesn't cover a directive", c.src[p.Span.Start:p.Span.End])
				}
			}
			if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestRefs(t *testing.T) {
	src := `@extends('layouts.app')
@section('title', 'Home')
@include('partials.nav', ['a' => 1])
@includeWhen($show, 'partials.flash')
@includeIf('maybe')
@include($dynamic)
@include("rows.$type")
@push('scripts')
@stack('styles')
@each('items.row', $items, 'item', 'raw|<p>none</p>')`
	var got []string
	for _, r := range Refs(src, Parse(src)) {
		if src[r.Span.Start:r.Span.End] != r.Name {
			t.Fatalf("span of %q covers %q", r.Name, src[r.Span.Start:r.Span.End])
		}
		s := r.Directive + ":" + r.Name
		if r.Declares {
			s += "!"
		}
		if r.Optional {
			s += "?"
		}
		got = append(got, s)
	}
	want := "extends:layouts.app section:title include:partials.nav includeWhen:partials.flash includeIf:maybe? push:scripts stack:styles! each:items.row each:raw|<p>none</p>"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
}

func TestPartialRefAt(t *testing.T) {
	cases := []struct {
		src, name string // | marks the cursor
		ok        bool
		kind      RefKind
	}{
		{"@include('partials.n|", "partials.n", true, RefView},
		{"@include('partials.n|av')", "partials.n", true, RefView},
		{"@includeWhen($x, 'pa|", "pa", true, RefView},
		{"@includeWhen('pa|", "", false, 0},
		{"@include('a', ['k' => 'v|", "", false, 0},
		{"@section('ti|", "ti", true, RefSection},
		{"@pushIf($c, 'sc|", "sc", true, RefStack},
		{"me@include('x|", "", false, 0},
		{"@include(|", "", false, 0},
	}
	for _, c := range cases {
		offset := strings.IndexByte(c.src, '|')
		src := c.src[:offset] + c.src[offset+1:]
		ref, ok := PartialRefAt(src, offset)
		if ok != c.ok || (ok && (ref.Name != c.name || ref.Kind != c.kind)) {
			t.Errorf("%s: got %+v %v", c.src, ref, ok)
		}
	}
}

func TestDirectivePrefixAt(t *testing.T) {
	cases := []struct {
		src, prefix string
		ok          bool
	}{
		{"<div>@fo|", "fo", true},
		{"<div>@|", "", true},
		{"<div>@foreach|", "foreach", true},
		{"me@exa|", "", false},
		{"@@fo|", "", false},
		{"{{ $a @fo| }}", "", false},
		{"@if($a @fo|)", "", false},
	}
	for _, c := range cases {
		offset := strings.IndexByte(c.src, '|')
		src := c.src[:offset] + c.src[offset+1:]
		_, prefix, ok := DirectivePrefixAt(src, Parse(src), offset)
		if ok != c.ok || prefix != c.prefix {
			t.Errorf("%s: got %q %v", c.src, prefix, ok)
		}
	}
}

type fakeViews map[string]string // name → source

func (v fakeViews) ViewURI(name string) (string, bool) {
	if _, ok := v[name]; ok {
		return "file:///views/" + name, true
	}
	return "", false
}

func (v fakeViews) ViewNames(prefix string) []string {
	var names []string
	for n := range v {
		if strings.HasPrefix(n, prefix) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func (v fakeViews) ViewsKnown() bool { return true }

func (v fakeViews) NamespaceKnown(ns string) bool { return ns == "pkg" }

func (v fakeViews) features() *Features {
	return &Features{Views: v, Read: func(uri string) string { return v[strings.TrimPrefix(uri, "file:///views/")] }}
}

func TestLayoutDeclarations(t *testing.T) {
	views := fakeViews{
		"layouts.app":   "@extends('layouts.base')<title>@yield('title')</title>@include('partials.head')@yield('content')",
		"layouts.base":  "@yield('meta') @stack('base-scripts')",
		"partials.head": "@stack('styles')",
		"loop":          "@extends('loop')@yield('never')",
	}
	page := "@extends('layouts.app') @section('title', 'Home')"
	var sections, stacks []string
	for _, d := range LayoutDeclarations("file:///views/page", page, RefSection, views.features().load) {
		sections = append(sections, d.Name)
	}
	for _, d := range LayoutDeclarations("file:///views/page", page, RefStack, views.features().load) {
		stacks = append(stacks, d.Name)
	}
	if strings.Join(sections, " ") != "title content meta" {
		t.Errorf("sections: %v", sections)
	}
	if strings.Join(stacks, " ") != "base-scripts styles" {
		t.Errorf("stacks: %v", stacks)
	}
	// A view that extends itself ends.
	_ = LayoutDeclarations("file:///views/loop", views["loop"], RefSection, views.features().load)
}

func TestFeaturesCompleteAndDefinition(t *testing.T) {
	views := fakeViews{
		"layouts.app":  "<title>@yield('title')</title>\n@yield('content')",
		"partials.nav": "nav",
	}
	f := views.features()

	src := "@extends('layouts.app')\n@section('c"
	items, ok := f.Complete("file:///views/page", src, len(src))
	if !ok || len(items) != 1 || items[0].Label != "content" {
		t.Fatalf("section completion: %+v %v", items, ok)
	}

	src = "@include('partials.')"
	items, ok = f.Complete("file:///views/page", src, len("@include('partials."))
	if !ok || len(items) != 1 || items[0].Insert != "partials.nav" || src[items[0].Start:items[0].End] != "partials." {
		t.Fatalf("view completion: %+v %v", items, ok)
	}

	src = "<p>@forea"
	items, ok = f.Complete("file:///views/page", src, len(src))
	if !ok || len(items) != 1 || !strings.HasSuffix(items[0].Insert, "@endforeach") || src[items[0].Start:items[0].End] != "@forea" {
		t.Fatalf("directive completion: %+v %v", items, ok)
	}

	// PHP is left to PHP completion.
	src = "{{ $user-> }}"
	if _, ok := f.Complete("file:///views/page", src, len("{{ $user->")); ok {
		t.Fatal("PHP in an echo should be left to PHP completion")
	}

	src = "@extends('layouts.app')\n@section('content')x@endsection"
	locs, ok := f.Definition("file:///views/page", src, strings.Index(src, "content")+1)
	if !ok || len(locs) != 1 || locs[0].URI != "file:///views/layouts.app" || locs[0].Line != 1 || locs[0].Col != len("@yield('") {
		t.Fatalf("section definition: %+v %v", locs, ok)
	}
	locs, ok = f.Definition("file:///views/page", src, strings.Index(src, "layouts")+1)
	if !ok || len(locs) != 1 || locs[0].URI != "file:///views/layouts.app" {
		t.Fatalf("view definition: %+v %v", locs, ok)
	}
}

func TestFeaturesHover(t *testing.T) {
	f := fakeViews{"partials.nav": ""}.features()
	src := "@foreach($a as $b) @include('partials.nav') @endforeach"
	h, ok := f.Hover(src, 3)
	if !ok || !strings.Contains(h.Markdown, "$loop") || src[h.Span.Start:h.Span.End] != "@foreach" {
		t.Fatalf("directive hover: %+v %v", h, ok)
	}
	h, ok = f.Hover(src, strings.Index(src, "nav"))
	if !ok || !strings.Contains(h.Markdown, "partials.nav") {
		t.Fatalf("view hover: %+v %v", h, ok)
	}
}

func TestFeaturesDiagnose(t *testing.T) {
	f := fakeViews{"partials.nav": ""}.features()
	src := `@include('partials.nav')
@include('partials.missing')
@includeIf('optional.missing')
@include('mail::message')
@include($dynamic)
@each('partials.nav', $items, 'item', 'raw|none')
@if($a)`
	var got []string
	for _, d := range f.Diagnose(src) {
		got = append(got, d.Code+": "+d.Message)
	}
	want := []string{
		"unclosed-directive: `@if` is never closed with `@endif`",
		"unknown-view: View `partials.missing` not found",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q", got)
	}
}

func TestEveryDirectiveIsDocumented(t *testing.T) {
	for name := range directives {
		if _, ok := LookupDirectiveInfo(name); !ok {
			switch name {
			case "includeisolated", "extendsfirst", "json", "filamentstyles", "filamentscripts", "routes", "inertia",
				"inertiahead", "fluxappearance", "fluxscripts", "endlang", "endcomponentclass":
				continue
			}
			t.Errorf("@%s has no documentation", name)
		}
	}
}
