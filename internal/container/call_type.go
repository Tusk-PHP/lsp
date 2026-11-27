package container

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// CallTypes returns a resolve.Engine ContainerCall hook backed by ca: the
// class a container call such as app('request'), app(Request::class),
// resolve('cache') or $container->get('log') resolves to. config() with no
// arguments is the Repository; config('key') gives a value, not an object,
// which the hook reports as "-" so resolution stops.
func CallTypes(ca *ContainerAnalyzer, index *symbols.Index) func(r *resolve.Resolver, expr, source string, file *parser.FileNode) string {
	return func(r *resolve.Resolver, expr, source string, file *parser.FileNode) string {
		t := strings.TrimSpace(expr)
		if t == "config()" {
			if binding := ca.ResolveDependency("config"); binding != nil {
				return binding.Concrete
			}
			return "Illuminate\\Config\\Repository"
		}
		if strings.HasPrefix(t, "config(") && strings.HasSuffix(t, ")") {
			return "-"
		}

		arg := resolve.ExtractContainerCallArg(expr)
		if arg == "" {
			return ""
		}
		// "Request::class" names a class: resolve it in the file.
		if strings.HasSuffix(arg, "::class") {
			arg = r.ResolveClassNameLoose(strings.TrimSuffix(arg, "::class"), file)
		}
		if binding := ca.ResolveDependency(arg); binding != nil {
			return binding.Concrete
		}
		// An argument that is already a known class.
		if index.Lookup(arg) != nil {
			return arg
		}
		return ""
	}
}
