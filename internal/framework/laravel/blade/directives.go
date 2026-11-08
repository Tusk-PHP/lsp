package blade

import "strings"

// directiveKind says how a directive becomes PHP.
type directiveKind int

const (
	// dirArgs has no structure: its arguments are PHP expressions, checked as
	// they are (`@include('view', ['a' => $b])`, `@section('title')`).
	dirArgs    directiveKind = iota
	dirIf                    // opens a conditional; its argument is the condition
	dirElse                  // `@else`
	dirElseIf                // `@elseif(cond)`, `@elseauth`, `@elsecan(...)`
	dirEndIf                 // closes a conditional
	dirForeach               // `@foreach($a as $b)`
	dirForelse               // `@forelse($a as $b)`, whose `@empty` is its else branch
	dirEndForeach
	dirEndForelse
	dirFor
	dirEndFor
	dirWhile
	dirEndWhile
	dirSwitch
	dirCase
	dirDefault
	dirEndSwitch
	dirBreak // `@break`, `@break($cond)`
	dirContinue
	dirUse  // `@use('App\Models\User')`
	dirPHP  // `@php`, read by the scanner
	dirNone // known, with nothing to check: `@csrf`, `@endsection`
)

// condition says how a conditional directive's arguments become a condition.
type condition int

const (
	condExpr   condition = iota // `@if($x)`: the argument is the condition
	condNot                     // `@unless($x)`
	condIsset                   // `@isset($x)`
	condEmpty                   // `@empty($x)`
	condOpaque                  // `@auth`, `@can('edit', $post)`: arguments are only checked
)

type directive struct {
	kind directiveKind
	cond condition
}

// directives lists the directives of Laravel and Livewire. Anything else after
// an `@` stays text, as Blade leaves it, so Alpine's `@click` and directives a
// project registers itself are never checked as PHP.
var directives = map[string]directive{}

func init() {
	add := func(kind directiveKind, cond condition, names ...string) {
		for _, n := range names {
			directives[strings.ToLower(n)] = directive{kind: kind, cond: cond}
		}
	}
	add(dirIf, condExpr, "if")
	add(dirIf, condNot, "unless")
	add(dirIf, condIsset, "isset")
	add(dirIf, condEmpty, "empty")
	add(dirIf, condOpaque, "auth", "guest", "production", "env", "can", "cannot", "canany",
		"error", "hasSection", "sectionMissing", "session", "context", "hasstack")
	add(dirElse, condExpr, "else")
	add(dirElseIf, condExpr, "elseif")
	add(dirElseIf, condOpaque, "elseauth", "elseguest", "elsecan", "elsecannot", "elsecanany")
	add(dirEndIf, condExpr, "endif", "endunless", "endisset", "endempty", "endauth", "endguest",
		"endproduction", "endenv", "endcan", "endcannot", "endcanany", "enderror", "endsession", "endcontext")
	add(dirForeach, condExpr, "foreach")
	add(dirEndForeach, condExpr, "endforeach")
	add(dirForelse, condExpr, "forelse")
	add(dirEndForelse, condExpr, "endforelse")
	add(dirFor, condExpr, "for")
	add(dirEndFor, condExpr, "endfor")
	add(dirWhile, condExpr, "while")
	add(dirEndWhile, condExpr, "endwhile")
	add(dirSwitch, condExpr, "switch")
	add(dirCase, condExpr, "case")
	add(dirDefault, condExpr, "default")
	add(dirEndSwitch, condExpr, "endswitch")
	add(dirBreak, condExpr, "break")
	add(dirContinue, condExpr, "continue")
	add(dirUse, condExpr, "use")
	add(dirPHP, condExpr, "php", "endphp", "verbatim", "endverbatim")
	add(dirArgs, condExpr,
		// Laravel
		"include", "includeIf", "includeWhen", "includeUnless", "includeFirst", "includeIsolated", "each",
		"extends", "extendsFirst", "section", "yield", "show", "stop", "overwrite", "append", "parent",
		"push", "pushOnce", "pushIf", "prepend", "prependOnce", "stack", "once", "fragment",
		"component", "componentFirst", "slot", "props", "aware", "inject",
		"json", "js", "class", "style", "checked", "selected", "disabled", "readonly", "required",
		"method", "csrf", "lang", "choice", "dd", "dump", "vite", "viteReactRefresh", "unset",
		// Livewire
		"livewire", "livewireStyles", "livewireScripts", "livewireScriptConfig", "persist", "entangle",
		"this", "script", "assets", "teleport",
		// Common packages
		"filamentStyles", "filamentScripts", "routes", "inertia", "inertiaHead", "fluxAppearance", "fluxScripts",
	)
	add(dirNone, condExpr,
		"endsection", "endpush", "endPushOnce", "endPushIf", "endprepend", "endPrependOnce", "endonce",
		"endfragment", "endcomponent", "endcomponentClass", "endslot", "endlang", "endpersist",
		"endscript", "endassets", "endteleport", "endcomponentFirst",
	)
}

func isDirective(name string) bool {
	_, ok := directives[strings.ToLower(name)]
	return ok
}

func lookupDirective(name string) directive {
	return directives[strings.ToLower(name)]
}
