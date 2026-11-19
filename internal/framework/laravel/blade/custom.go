package blade

import "strings"

// Custom is what a project registers with Blade itself: directives
// (`Blade::directive('money', …)`) and conditionals (`Blade::if('admin',
// …)`), by lower-case name. A nil Custom is none.
type Custom struct {
	Directives   map[string]bool
	Conditionals map[string]bool
	// Where locates each registration, by lower-case name.
	Where map[string]Location
	// Names are the names as registered, by lower-case name.
	Names map[string]string
}

// name returns a directive's name as registered.
func (c *Custom) name(lower string) string {
	if n, ok := c.Names[lower]; ok {
		return n
	}
	return lower
}

// directive returns how a custom directive becomes PHP: a conditional's
// `@admin`, `@unlessadmin`, `@elseadmin` and `@endadmin`, or a directive
// whose arguments aren't read as PHP, since its handler decides what they
// mean.
func (c *Custom) directive(name string) (directive, bool) {
	if c == nil {
		return directive{}, false
	}
	n := strings.ToLower(name)
	if c.Directives[n] {
		return directive{kind: dirNone}, true
	}
	switch {
	case c.Conditionals[n]:
		return directive{kind: dirIf, cond: condOpaque}, true
	case strings.HasPrefix(n, "unless") && c.Conditionals[n[len("unless"):]]:
		return directive{kind: dirIf, cond: condOpaque}, true
	case strings.HasPrefix(n, "else") && c.Conditionals[n[len("else"):]]:
		return directive{kind: dirElseIf, cond: condOpaque}, true
	case strings.HasPrefix(n, "end") && c.Conditionals[n[len("end"):]]:
		return directive{kind: dirEndIf}, true
	}
	return directive{}, false
}

// conditionalOf returns the conditional a custom directive belongs to.
func (c *Custom) conditionalOf(name string) string {
	if c == nil {
		return ""
	}
	n := strings.ToLower(name)
	for _, p := range []string{"", "unless", "else", "end"} {
		if strings.HasPrefix(n, p) && c.Conditionals[n[len(p):]] {
			return n[len(p):]
		}
	}
	return ""
}

func (c *Custom) known(name string) bool {
	_, ok := c.directive(name)
	return ok
}
