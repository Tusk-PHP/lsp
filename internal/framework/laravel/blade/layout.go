package blade

import "strings"

// LoadView returns the URI and source of a view by name, when it exists.
type LoadView func(name string) (uri, src string, ok bool)

// Declaration is a `@yield` or `@stack` a view's layouts declare.
type Declaration struct {
	URI       string
	Name      string
	Line, Col int // where the name starts, 0-based, byte column
	EndCol    int
}

// maxLayoutDepth bounds how far `@extends` is followed, so a cycle or a deep
// chain can't make a request slow.
const maxLayoutDepth = 8

// LayoutDeclarations returns the sections (`@yield`) or stacks (`@stack`) of
// kind that the layouts of a view declare: the view it extends, the views
// those extend, and the views each layout includes, where stacks often live.
// The view's own declarations come first.
func LayoutDeclarations(uri, src string, kind RefKind, load LoadView) []Declaration {
	var out []Declaration
	seen := map[string]bool{uri: true}
	var walk func(uri, src string, depth int, followIncludes bool)
	walk = func(uri, src string, depth int, followIncludes bool) {
		refs := Refs(src, Parse(src))
		var lines Lines
		for _, r := range refs {
			if r.Kind != kind || !r.Declares {
				continue
			}
			if lines == nil {
				lines = NewLines(src)
			}
			line, col := lines.Position(r.Span.Start)
			out = append(out, Declaration{URI: uri, Name: r.Name, Line: line, Col: col, EndCol: col + len(r.Name)})
		}
		if depth >= maxLayoutDepth {
			return
		}
		for _, r := range refs {
			if r.Kind != RefView {
				continue
			}
			extends := strings.EqualFold(r.Directive, "extends")
			if !extends && !(followIncludes && strings.HasPrefix(strings.ToLower(r.Directive), "include")) {
				continue
			}
			luri, lsrc, ok := load(r.Name)
			if !ok || seen[luri] {
				continue
			}
			seen[luri] = true
			// A layout's includes are searched, not the includes of those.
			walk(luri, lsrc, depth+1, extends)
		}
	}
	walk(uri, src, 0, false)
	return out
}
