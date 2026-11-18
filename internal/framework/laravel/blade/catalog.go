package blade

import (
	"sort"
	"strings"
)

// DirectiveInfo describes a directive for completion and hover.
type DirectiveInfo struct {
	Name string // as written, without `@`: `foreach`, `endPushOnce`
	// Doc says what the directive does, in Markdown.
	Doc string
	// Snippet is what completion inserts after `@`, in LSP snippet syntax,
	// with the closing directive for a block.
	Snippet string
	// Closer is the directive that ends a block opened by this one, if any.
	Closer string
}

// catalogEntry is {name, arguments snippet, closer, doc}.
type catalogEntry [4]string

var catalogEntries = []catalogEntry{
	// Conditionals
	{"if", "(${1:condition})", "endif", "Renders its body when the condition is true. Takes `@elseif` and `@else` branches."},
	{"elseif", "(${1:condition})", "", "Another branch of an `@if`, rendered when its condition is true and no earlier one was."},
	{"else", "", "", "The branch of a conditional rendered when no other branch was."},
	{"endif", "", "", "Ends an `@if` block."},
	{"unless", "(${1:condition})", "endunless", "Renders its body when the condition is false."},
	{"endunless", "", "", "Ends an `@unless` block."},
	{"isset", "(${1:\\$variable})", "endisset", "Renders its body when the variable is set and not null."},
	{"endisset", "", "", "Ends an `@isset` block."},
	{"empty", "(${1:\\$variable})", "endempty", "Renders its body when the variable is empty. Without arguments inside `@forelse`, starts the branch rendered when the loop has nothing to iterate."},
	{"endempty", "", "", "Ends an `@empty` block."},
	{"auth", "", "endauth", "Renders its body when the user is signed in, optionally with the guard given: `@auth('admin')`."},
	{"elseauth", "", "", "A branch rendered when the user is signed in with the guard given."},
	{"endauth", "", "", "Ends an `@auth` block."},
	{"guest", "", "endguest", "Renders its body when no user is signed in, optionally for the guard given."},
	{"elseguest", "", "", "A branch rendered when no user is signed in with the guard given."},
	{"endguest", "", "", "Ends a `@guest` block."},
	{"production", "", "endproduction", "Renders its body when the application runs in production."},
	{"endproduction", "", "", "Ends a `@production` block."},
	{"env", "('${1:local}')", "endenv", "Renders its body when the application runs in the environment given, or one of a list."},
	{"endenv", "", "", "Ends an `@env` block."},
	{"can", "('${1:ability}', ${2:\\$model})", "endcan", "Renders its body when the user is authorized for the ability."},
	{"elsecan", "('${1:ability}', ${2:\\$model})", "", "A branch rendered when the user is authorized for the ability."},
	{"endcan", "", "", "Ends a `@can` block."},
	{"cannot", "('${1:ability}', ${2:\\$model})", "endcannot", "Renders its body when the user isn't authorized for the ability."},
	{"elsecannot", "('${1:ability}', ${2:\\$model})", "", "A branch rendered when the user isn't authorized for the ability."},
	{"endcannot", "", "", "Ends a `@cannot` block."},
	{"canany", "([${1:'update', 'delete'}], ${2:\\$model})", "endcanany", "Renders its body when the user is authorized for any of the abilities."},
	{"elsecanany", "([${1:'update', 'delete'}], ${2:\\$model})", "", "A branch rendered when the user is authorized for any of the abilities."},
	{"endcanany", "", "", "Ends a `@canany` block."},
	{"error", "('${1:field}')", "enderror", "Renders its body when the field has a validation error, available in it as `$message`."},
	{"enderror", "", "", "Ends an `@error` block."},
	{"hasSection", "('${1:section}')", "endif", "Renders its body when the section has content."},
	{"sectionMissing", "('${1:section}')", "endif", "Renders its body when the section has no content."},
	{"session", "('${1:key}')", "endsession", "Renders its body when the session has a value for the key, available in it as `$value`."},
	{"endsession", "", "", "Ends a `@session` block."},
	{"context", "('${1:key}')", "endcontext", "Renders its body when the context has a value for the key, available in it as `$value`."},
	{"endcontext", "", "", "Ends a `@context` block."},
	{"switch", "(${1:\\$value})", "endswitch", "Renders the `@case` matching the value, or `@default`."},
	{"case", "(${1:value})", "", "A case of a `@switch`. End it with `@break`."},
	{"default", "", "", "The case of a `@switch` rendered when no other case matches."},
	{"endswitch", "", "", "Ends a `@switch` block."},

	// Loops
	{"foreach", "(${1:\\$items} as ${2:\\$item})", "endforeach", "Renders its body for each item. `$loop` holds the iteration: `$loop->index`, `$loop->first`, `$loop->last` and more."},
	{"endforeach", "", "", "Ends a `@foreach` block."},
	{"forelse", "(${1:\\$items} as ${2:\\$item})", "endforelse", "Renders its body for each item, and its `@empty` branch when there are none."},
	{"endforelse", "", "", "Ends a `@forelse` block."},
	{"for", "(${1:\\$i = 0}; ${2:\\$i < 10}; ${3:\\$i++})", "endfor", "A PHP `for` loop."},
	{"endfor", "", "", "Ends a `@for` block."},
	{"while", "(${1:condition})", "endwhile", "Renders its body while the condition is true."},
	{"endwhile", "", "", "Ends a `@while` block."},
	{"break", "", "", "Leaves the loop, or ends a `@case`. With a condition, only when it's true: `@break($loop->index > 5)`."},
	{"continue", "", "", "Skips to the next iteration. With a condition, only when it's true."},

	// Layouts and views
	{"extends", "('${1:layouts.app}')", "", "Renders this view inside the layout given, filling its `@yield`s with this view's `@section`s."},
	{"section", "('${1:name}')", "endsection", "Defines the content of a section, shown where the layout has `@yield` for it. `@section('title', 'Text')` sets it in one line."},
	{"endsection", "", "", "Ends a `@section` block."},
	{"show", "", "", "Ends a `@section` block and shows it right away, so views that extend this one can replace it."},
	{"stop", "", "", "Ends a `@section` block."},
	{"append", "", "", "Ends a `@section` block, adding its content to the section's existing content."},
	{"overwrite", "", "", "Ends a `@section` block, replacing the section's content."},
	{"yield", "('${1:name}')", "", "Shows the content of a section, or the default given as second argument."},
	{"parent", "", "", "Inside a `@section`, shows the content the layout gave the section."},
	{"include", "('${1:view}')", "", "Renders another view here. It gets this view's variables, plus any given in an array."},
	{"includeIf", "('${1:view}')", "", "Renders another view here if it exists."},
	{"includeWhen", "(${1:condition}, '${2:view}')", "", "Renders another view here when the condition is true."},
	{"includeUnless", "(${1:condition}, '${2:view}')", "", "Renders another view here when the condition is false."},
	{"includeFirst", "(['${1:view}'])", "", "Renders the first view of the list that exists."},
	{"each", "('${1:view}', ${2:\\$items}, '${3:item}')", "", "Renders a view for each item of a collection, with an optional view for when it's empty."},
	{"push", "('${1:stack}')", "endpush", "Adds content to a stack, shown where a layout has `@stack` for it."},
	{"endpush", "", "", "Ends a `@push` block."},
	{"pushOnce", "('${1:stack}')", "endPushOnce", "Adds content to a stack once per rendering, however often the view is rendered."},
	{"endPushOnce", "", "", "Ends a `@pushOnce` block."},
	{"pushIf", "(${1:condition}, '${2:stack}')", "endPushIf", "Adds content to a stack when the condition is true."},
	{"endPushIf", "", "", "Ends a `@pushIf` block."},
	{"prepend", "('${1:stack}')", "endprepend", "Adds content to the start of a stack."},
	{"endprepend", "", "", "Ends a `@prepend` block."},
	{"prependOnce", "('${1:stack}')", "endPrependOnce", "Adds content to the start of a stack once per rendering."},
	{"endPrependOnce", "", "", "Ends a `@prependOnce` block."},
	{"stack", "('${1:name}')", "", "Shows the content pushed to a stack."},
	{"hasstack", "('${1:name}')", "endif", "Renders its body when the stack has content."},
	{"once", "", "endonce", "Renders its body only once per rendering, however often the view is rendered."},
	{"endonce", "", "", "Ends a `@once` block."},
	{"fragment", "('${1:name}')", "endfragment", "Marks part of the view that can be rendered on its own with `fragment()`."},
	{"endfragment", "", "", "Ends a `@fragment` block."},

	// Components
	{"component", "('${1:view}')", "endcomponent", "Renders a component view, with the block's content as `$slot`."},
	{"endcomponent", "", "", "Ends a `@component` block."},
	{"componentFirst", "(['${1:view}'])", "endcomponentFirst", "Renders the first component view of the list that exists."},
	{"endcomponentFirst", "", "", "Ends a `@componentFirst` block."},
	{"slot", "('${1:name}')", "endslot", "Fills a named slot of a `@component`."},
	{"endslot", "", "", "Ends a `@slot` block."},
	{"props", "([${1:'name'}])", "", "Declares the properties of an anonymous component, with defaults as `'name' => value`. Other attributes stay in `$attributes`."},
	{"aware", "([${1:'name'}])", "", "Reads properties from the parent component."},

	// PHP and output
	{"php", "", "endphp", "Runs a block of PHP. `@php($expression)` runs one statement."},
	{"endphp", "", "", "Ends a `@php` block."},
	{"verbatim", "", "endverbatim", "Shows its body as it is, without compiling Blade in it."},
	{"endverbatim", "", "", "Ends a `@verbatim` block."},
	{"use", "('${1:App\\\\Models\\\\User}')", "", "Imports a class for the view, optionally with an alias as second argument."},
	{"inject", "('${1:name}', ${2:Service::class})", "", "Resolves a service from the container into a variable."},
	{"json", "(${1:\\$data})", "", "Prints the value encoded as JSON."},
	{"js", "(${1:\\$data})", "", "Prints the value as a JavaScript expression."},
	{"class", "([${1:'active' => \\$isActive}])", "", "Prints a `class` attribute from the classes whose condition is true."},
	{"style", "([${1:'color: red' => \\$isError}])", "", "Prints a `style` attribute from the styles whose condition is true."},
	{"checked", "(${1:condition})", "", "Prints `checked` when the condition is true."},
	{"selected", "(${1:condition})", "", "Prints `selected` when the condition is true."},
	{"disabled", "(${1:condition})", "", "Prints `disabled` when the condition is true."},
	{"readonly", "(${1:condition})", "", "Prints `readonly` when the condition is true."},
	{"required", "(${1:condition})", "", "Prints `required` when the condition is true."},
	{"csrf", "", "", "Prints a hidden input with the CSRF token."},
	{"method", "('${1:PUT}')", "", "Prints a hidden input that makes a form submit with the HTTP method given."},
	{"lang", "('${1:key}')", "", "Prints a translation."},
	{"choice", "('${1:key}', ${2:\\$count})", "", "Prints a translation in the plural form for the count."},
	{"dd", "(${1:\\$value})", "", "Dumps the values and stops rendering."},
	{"dump", "(${1:\\$value})", "", "Dumps the values."},
	{"vite", "(['${1:resources/js/app.js}'])", "", "Prints the tags that load the Vite entry points given."},
	{"viteReactRefresh", "", "", "Prints the script that enables React fast refresh with Vite."},
	{"unset", "(${1:\\$variable})", "", "Unsets a variable."},

	// Livewire
	{"livewire", "('${1:component}')", "", "Renders a Livewire component."},
	{"livewireStyles", "", "", "Prints Livewire's styles."},
	{"livewireScripts", "", "", "Prints Livewire's scripts."},
	{"livewireScriptConfig", "", "", "Prints Livewire's script configuration, for when its JavaScript is bundled."},
	{"persist", "('${1:name}')", "endpersist", "Keeps the element alive across `wire:navigate` page visits."},
	{"endpersist", "", "", "Ends a `@persist` block."},
	{"teleport", "('${1:body}')", "endteleport", "Renders its body in another part of the page."},
	{"endteleport", "", "", "Ends a `@teleport` block."},
	{"script", "", "endscript", "JavaScript run with the Livewire component, with `$wire` available."},
	{"endscript", "", "", "Ends a `@script` block."},
	{"assets", "", "endassets", "Scripts and styles loaded once, the first time the component appears on the page."},
	{"endassets", "", "", "Ends an `@assets` block."},
	{"entangle", "('${1:property}')", "", "Binds an Alpine value to a Livewire property."},
	{"this", "", "", "Refers to the Livewire component from Alpine."},
}

var catalog = func() map[string]DirectiveInfo {
	m := make(map[string]DirectiveInfo, len(catalogEntries))
	for _, e := range catalogEntries {
		info := DirectiveInfo{Name: e[0], Closer: e[2], Doc: e[3]}
		info.Snippet = e[0] + e[1]
		if e[2] != "" {
			info.Snippet += "\n\t$0\n@" + e[2]
		}
		m[strings.ToLower(e[0])] = info
	}
	return m
}()

// LookupDirectiveInfo returns what is known about a directive, by name
// without `@`, in any case, as Blade reads them.
func LookupDirectiveInfo(name string) (DirectiveInfo, bool) {
	info, ok := catalog[strings.ToLower(name)]
	return info, ok
}

// DirectiveInfos returns the documented directives, sorted by name.
func DirectiveInfos() []DirectiveInfo {
	out := make([]DirectiveInfo, 0, len(catalog))
	for _, info := range catalog {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
