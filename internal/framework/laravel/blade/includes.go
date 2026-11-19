package blade

import "strings"

// Include is a view a template renders inside it, and the data it passes.
type Include struct {
	View string
	// Isolated is set for `@each`, whose view gets only its item, not the
	// variables of the template.
	Isolated bool
	Vars     []IncludeVar
}

// IncludeVar is a variable a template passes to a view it includes: a key of
// its data array, or the item of `@each`.
type IncludeVar struct {
	Name  string
	Key   Span // the name, without quotes
	Value Span // the PHP expression passed; empty when there is none
}

// includeArgs says, by directive, which argument names the view and which
// passes its data.
var includeArgs = map[string][2]int{
	"include":       {0, 1},
	"includeif":     {0, 1},
	"includewhen":   {1, 2},
	"includeunless": {1, 2},
	"extends":       {0, 1},
	"component":     {0, 1},
}

// Includes returns the views a template renders inside it with a name it
// writes as a string: `@include`, `@includeIf`, `@includeWhen`,
// `@includeUnless`, `@extends`, `@component` and `@each`.
func Includes(src string, nodes []Node) []Include {
	var out []Include
	for _, n := range nodes {
		if n.Kind != KindDirective || !n.HasArgs {
			continue
		}
		name := strings.ToLower(n.Name)
		args := splitArgs(src, n.Args)
		if name == "each" {
			// @each('view', $items, 'item'): the view gets $item and $key.
			if len(args) < 3 {
				continue
			}
			view, ok1 := stringLiteral(src, args[0])
			item, ok2 := stringLiteral(src, args[2])
			if !ok1 || !ok2 {
				continue
			}
			out = append(out, Include{View: src[view.Start:view.End], Isolated: true, Vars: []IncludeVar{
				{Name: src[item.Start:item.End], Key: item},
				{Name: "key", Key: item},
			}})
			continue
		}
		at, ok := includeArgs[name]
		if !ok || at[0] >= len(args) {
			continue
		}
		view, ok := stringLiteral(src, args[at[0]])
		if !ok {
			continue
		}
		inc := Include{View: src[view.Start:view.End]}
		if at[1] < len(args) {
			inc.Vars = arrayVars(src, args[at[1]])
		}
		out = append(out, inc)
	}
	return out
}

// arrayVars reads the `'key' => expr` items of an array literal.
func arrayVars(src string, arg Span) []IncludeVar {
	arr := trimSpan(src, arg)
	if arr.End-arr.Start < 2 || src[arr.Start] != '[' || src[arr.End-1] != ']' {
		return nil
	}
	var vars []IncludeVar
	for _, item := range splitArgs(src, Span{arr.Start + 1, arr.End - 1}) {
		k := topLevelArrow(src, item)
		if k < 0 {
			continue
		}
		key, ok := stringLiteral(src, Span{item.Start, k})
		if !ok {
			continue
		}
		vars = append(vars, IncludeVar{Name: src[key.Start:key.End], Key: key, Value: trimSpan(src, Span{k + 2, item.End})})
	}
	return vars
}
