package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// Receiver is the receiver of the member access at a position: its type,
// whether the access is static (::), and whether it's self::, parent:: or
// static::, which keep the calling class for late static binding.
type Receiver struct {
	Type           ResolvedType
	Static         bool
	ForwardsCaller bool
}

// ReceiverAt finds the member access at pos in the file, for "$a->b()->c"
// with pos on c or just after the last ->, and types its receiver, here what
// $a->b() gives; for "Foo::" it's Foo. ok is false when pos isn't in a member
// access.
func (r *Resolver) ReceiverAt(source string, pos textpos.Position, file *parser.FileNode) (Receiver, bool) {
	if file == nil {
		return Receiver{}, false
	}
	lines := r.Lines(source)
	t := r.syntaxTree(lines, file)
	receiver, _, static := ast.MemberAccess(ast.MemberAccessAt(t.root, t.offset(pos)))
	if receiver == nil {
		return Receiver{}, false
	}
	return Receiver{
		Type:           r.typeOf(t, receiver, nil, ExprContext{Lines: lines, Pos: pos, File: file}),
		Static:         static,
		ForwardsCaller: static && forwardsCallingClass(t, receiver),
	}, true
}

// CallTypeAt is what the method call at pos gives, for pos on the method's
// name. It's empty when pos isn't on the name of a method call.
func (r *Resolver) CallTypeAt(source string, pos textpos.Position, file *parser.FileNode) ResolvedType {
	if file == nil {
		return ResolvedType{}
	}
	lines := r.Lines(source)
	t := r.syntaxTree(lines, file)
	switch access := ast.MemberAccessAt(t.root, t.offset(pos)).(type) {
	case *ast.MethodCall, *ast.StaticCall:
		return r.typeOf(t, access, nil, ExprContext{Lines: lines, Pos: pos, File: file})
	}
	return ResolvedType{}
}

// ReceiverTypeAt is the type of ReceiverAt's receiver.
func (r *Resolver) ReceiverTypeAt(source string, pos textpos.Position, file *parser.FileNode) ResolvedType {
	recv, _ := r.ReceiverAt(source, pos, file)
	return recv.Type
}

func (r *Resolver) NodeType(file *parser.FileNode, e ast.Expr) ResolvedType {
	lines := r.Lines(file.Source)
	t := r.syntaxTree(lines, file)
	line, col := t.lines.Position(e.Bounds().Start)
	return r.typeOf(t, e, nil, ExprContext{Lines: lines, Pos: textpos.Position{Line: line, Character: col}, File: file})
}

// ClassKeyword lower-cases target when it's one of the class keywords
// self, static or parent, so SELF:: and Parent:: resolve like their lower-case
// forms; any other name is returned as it is.
func ClassKeyword(target string) string {
	switch lower := strings.ToLower(target); lower {
	case "self", "static", "parent":
		return lower
	}
	return target
}

// resolveClassNameFromSource resolves a short or FQN class name using
// the source file's use statements and namespace.
func (r *Resolver) ResolveClassNameLoose(name string, file *parser.FileNode) string {
	if name == "" {
		return ""
	}
	// Already fully qualified
	if strings.HasPrefix(name, "\\") {
		fqn := strings.TrimPrefix(name, "\\")
		if r.Index.Lookup(fqn) != nil {
			return fqn
		}
		return fqn
	}

	if file == nil {
		if cls := r.classNamed(name); cls != nil {
			return cls.FQN
		}
		return name
	}

	// Check use statements
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range file.Uses {
		if u.Alias == parts[0] {
			if len(parts) > 1 {
				return u.FullName + "\\" + parts[1]
			}
			return u.FullName
		}
	}
	// Try in current namespace
	if file.Namespace != "" {
		fqn := file.Namespace + "\\" + name
		if r.Index.Lookup(fqn) != nil {
			return fqn
		}
	}
	// Try as global
	if r.Index.Lookup(name) != nil {
		return name
	}
	if cls := r.classNamed(name); cls != nil {
		return cls.FQN
	}
	return name
}

func (r *Resolver) classNamed(name string) *symbols.Symbol {
	var classes []*symbols.Symbol
	for _, sym := range r.Index.LookupByName(name) {
		if sym.IsClassLike() {
			classes = append(classes, sym)
		}
	}
	return symbols.PickBestStandalone(classes, name)
}

// DiagnosticType returns the type diagnostics check an expression node of
// file against: the generic-aware type, written out in full ("?Foo",
// "Foo|Bar", "Collection<int, User>"), the same one hover and completion
// see. Each lookup is a request of its own.
func (e *Engine) DiagnosticType(file *parser.FileNode, node ast.Expr) string {
	return e.Resolver().NodeType(file, node).FullString()
}
