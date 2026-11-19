package blade

import (
	"regexp"
	"strings"
)

// Problem is something wrong in a template, at a span of it.
type Problem struct {
	Span    Span
	Message string
}

// blockGroup is a kind of block: the directives that open it all close with
// any of its closers. Every conditional ends in PHP's `endif`, so `@auth`
// closed by `@endif` works, and isn't reported.
type blockGroup string

type blockRule struct {
	group blockGroup
	// inline reports whether the directive, with these arguments, is complete
	// on its own, as `@section('title', 'Home')` is.
	inline func(args []Span, src string) bool
}

var (
	// openers are the directives that open a block.
	openers = map[string]blockRule{}
	// closers are the directives that close one, by the group they close.
	closers = map[string]blockGroup{}
	// branches are the directives only valid inside a block of a group.
	branches = map[string]blockGroup{}
	// displayCloser is the closer named in messages, by group.
	displayCloser = map[blockGroup]string{}
)

func init() {
	block := func(group blockGroup, closer string, inline func([]Span, string) bool, names ...string) {
		for _, n := range names {
			openers[strings.ToLower(n)] = blockRule{group: group, inline: inline}
		}
		if _, ok := displayCloser[group]; !ok {
			displayCloser[group] = closer
		}
	}
	close := func(group blockGroup, names ...string) {
		for _, n := range names {
			closers[strings.ToLower(n)] = group
		}
	}
	twoArgs := func(args []Span, _ string) bool { return len(args) >= 2 }

	block("if", "endif", nil, "if", "unless", "isset", "auth", "guest", "production", "env", "can", "cannot",
		"canany", "error", "hasSection", "sectionMissing", "session", "context", "hasstack")
	// `@empty` without arguments is a branch of `@forelse`, handled apart.
	block("if", "endif", nil, "empty")
	close("if", "endif", "endunless", "endisset", "endempty", "endauth", "endguest", "endproduction", "endenv",
		"endcan", "endcannot", "endcanany", "enderror", "endsession", "endcontext")
	for _, n := range []string{"else", "elseif", "elseauth", "elseguest", "elsecan", "elsecannot", "elsecanany"} {
		branches[n] = "if"
	}
	block("foreach", "endforeach", nil, "foreach")
	close("foreach", "endforeach")
	block("forelse", "endforelse", nil, "forelse")
	close("forelse", "endforelse")
	block("for", "endfor", nil, "for")
	close("for", "endfor")
	block("while", "endwhile", nil, "while")
	close("while", "endwhile")
	block("switch", "endswitch", nil, "switch")
	close("switch", "endswitch")
	block("section", "endsection", twoArgs, "section")
	close("section", "endsection", "show", "stop", "append", "overwrite")
	block("push", "endpush", twoArgs, "push")
	close("push", "endpush")
	block("prepend", "endprepend", twoArgs, "prepend")
	close("prepend", "endprepend")
	block("pushOnce", "endPushOnce", nil, "pushOnce")
	close("pushOnce", "endPushOnce")
	block("prependOnce", "endPrependOnce", nil, "prependOnce")
	close("prependOnce", "endPrependOnce")
	block("pushIf", "endPushIf", nil, "pushIf")
	close("pushIf", "endPushIf")
	block("once", "endonce", nil, "once")
	close("once", "endonce")
	block("fragment", "endfragment", nil, "fragment")
	close("fragment", "endfragment")
	block("component", "endcomponent", nil, "component")
	close("component", "endcomponent")
	block("componentFirst", "endcomponentFirst", nil, "componentFirst")
	close("componentFirst", "endcomponentFirst")
	// `@slot('name', 'value')` is inline; `@slot('name', null, [...])` isn't.
	block("slot", "endslot", func(args []Span, src string) bool {
		return len(args) == 2 || (len(args) > 2 && strings.TrimSpace(src[args[1].Start:args[1].End]) != "null")
	}, "slot")
	close("slot", "endslot")
	block("persist", "endpersist", nil, "persist")
	close("persist", "endpersist")
	block("teleport", "endteleport", nil, "teleport")
	close("teleport", "endteleport")
	block("script", "endscript", nil, "script")
	close("script", "endscript")
	block("assets", "endassets", nil, "assets")
	close("assets", "endassets")
}

// customEnd finds `@end…` words, the closers of directives a project
// registers itself, such as `Blade::if('admin')`.
var customEnd = regexp.MustCompile(`@end[A-Za-z_]\w*`)

type openBlock struct {
	group blockGroup
	node  Node
	// closer names the closer in messages when it isn't the group's own,
	// as `@endadmin` for a conditional registered with Blade::if.
	closer string
}

// CheckStructure reports blocks that are never closed and closing directives
// that close nothing, such as `@if` without `@endif`. A conditional closed by
// another conditional's closer (`@auth … @endif`) runs, and isn't reported.
func CheckStructure(src string, nodes []Node) []Problem {
	return CheckStructureCustom(src, nodes, nil)
}

// customRole returns how a directive the project registers takes part in
// blocks: its conditionals as `@if`, and a directive registered with its
// `@end…` as a block of its own.
func customRole(c *Custom, name string) (opener *blockRule, branch, closer blockGroup) {
	if c == nil || isDirective(name) {
		return nil, "", ""
	}
	n := strings.ToLower(name)
	if cond := c.conditionalOf(n); cond != "" {
		switch {
		case n == cond || n == "unless"+cond:
			return &blockRule{group: "if"}, "", ""
		case n == "else"+cond:
			return nil, "if", ""
		default:
			return nil, "", "if"
		}
	}
	if c.Directives[n] && c.Directives["end"+n] {
		return &blockRule{group: blockGroup("custom:" + n)}, "", ""
	}
	if strings.HasPrefix(n, "end") && c.Directives[n] && c.Directives[n[3:]] {
		return nil, "", blockGroup("custom:" + n[3:])
	}
	return nil, "", ""
}

// CheckStructureCustom is CheckStructure knowing the directives and
// conditionals the project registers.
func CheckStructureCustom(src string, nodes []Node, custom *Custom) []Problem {
	// A project's own block directives aren't known, and their branches look
	// like `@else` outside a block: stay quiet about branches then.
	checkBranches := true
	for _, m := range customEnd.FindAllString(src, -1) {
		if !isDirective(m[1:]) && !custom.known(m[1:]) {
			checkBranches = false
			break
		}
	}
	var problems []Problem
	var stack []openBlock
	for _, n := range nodes {
		if n.Kind != KindDirective {
			continue
		}
		name := strings.ToLower(n.Name)
		if rule, branch, closer := customRole(custom, name); rule != nil || branch != "" || closer != "" {
			switch {
			case rule != nil:
				ob := openBlock{group: rule.group, node: n}
				if cond := custom.conditionalOf(name); cond != "" {
					ob.closer = "end" + custom.name(cond)
				}
				stack = append(stack, ob)
				continue
			case branch != "":
				if checkBranches && (len(stack) == 0 || stack[len(stack)-1].group != branch) {
					problems = append(problems, Problem{Span: nameSpan(n), Message: "`@" + n.Name + "` is outside an `@if` block"})
				}
				continue
			}
		}
		if rule, ok := openers[name]; ok {
			if name == "empty" && !n.HasArgs {
				// The `@empty` branch of a `@forelse`.
				if checkBranches && (len(stack) == 0 || stack[len(stack)-1].group != "forelse") {
					problems = append(problems, Problem{Span: nameSpan(n), Message: "`@empty` without arguments belongs inside `@forelse`"})
				}
				continue
			}
			if rule.inline != nil && n.HasArgs && rule.inline(splitArgs(src, n.Args), src) {
				continue
			}
			stack = append(stack, openBlock{group: rule.group, node: n})
			continue
		}
		if group, ok := branches[name]; ok {
			if checkBranches && (len(stack) == 0 || stack[len(stack)-1].group != group) {
				problems = append(problems, Problem{Span: nameSpan(n), Message: "`@" + n.Name + "` is outside an `@if` block"})
			}
			continue
		}
		group, ok := closers[name]
		if _, _, closer := customRole(custom, name); closer != "" {
			group, ok = closer, true
		}
		if !ok {
			continue
		}
		at := -1
		for k := len(stack) - 1; k >= 0; k-- {
			if stack[k].group == group {
				at = k
				break
			}
		}
		if at < 0 {
			problems = append(problems, Problem{Span: nameSpan(n), Message: "`@" + n.Name + "` has no opening directive to close"})
			continue
		}
		// Blocks opened after the one closed here were never closed.
		for k := len(stack) - 1; k > at; k-- {
			o := stack[k]
			problems = append(problems, Problem{
				Span:    nameSpan(o.node),
				Message: "`@" + o.node.Name + "` is not closed: `@" + n.Name + "` closes an outer block first",
			})
		}
		stack = stack[:at]
	}
	for _, o := range stack {
		problems = append(problems, Problem{
			Span:    nameSpan(o.node),
			Message: "`@" + o.node.Name + "` is never closed with `@" + o.closerName() + "`",
		})
	}
	return problems
}

// nameSpan is the `@name` of a directive.
func nameSpan(n Node) Span {
	return Span{n.Span.Start, n.Span.Start + 1 + len(n.Name)}
}

func (o openBlock) closerName() string {
	if o.closer != "" {
		return o.closer
	}
	return closerName(o.group)
}

// closerName is the closer named in messages for a group: a custom block's
// is its opener's `@end…`.
func closerName(g blockGroup) string {
	if name, ok := strings.CutPrefix(string(g), "custom:"); ok {
		return "end" + name
	}
	return displayCloser[g]
}
