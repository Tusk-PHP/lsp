package diagnostics

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
)

// BladeLayout lays out a Blade view for suppression comments and the quick
// fixes that write them: Blade's comments, the PHP of its @php blocks, and
// the directives and echoes that span lines, as framework/laravel/blade
// reads them. custom names the project's own directives.
func BladeLayout(source string, custom *blade.Custom) checks.Layout {
	t := checks.Template{CommentOpen: blade.CommentOpen, CommentClose: blade.CommentClose}
	for _, c := range blade.Comments(source) {
		t.Comments = append(t.Comments, checks.Span{Start: c.Start, End: c.End})
	}
	for _, n := range blade.ParseCustom(source, custom) {
		if (n.Kind == blade.KindDirective || n.Kind == blade.KindEcho) && strings.Contains(source[n.Span.Start:n.Span.End], "\n") {
			t.Constructs = append(t.Constructs, checks.Span{Start: n.Span.Start, End: n.Span.End})
		}
		if n.Kind == blade.KindPHP && !n.Raw {
			t.PHP = append(t.PHP, checks.Span{Start: n.Expr.Start, End: n.Expr.End})
		}
	}
	return checks.TemplateLayout(source, t)
}

// bladeSuppressions are the @tusk-ignore and @phpstan-ignore comments of a
// Blade view.
func bladeSuppressions(source string, custom *blade.Custom) checks.Suppressions {
	if !checks.HasSuppressionMarker(source) {
		return nil
	}
	return BladeLayout(source, custom).Suppressions()
}
