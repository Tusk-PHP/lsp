package checks

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestReferencesComeFromTheSyntaxTree(t *testing.T) {
	src := `<?php
namespace App;
#[Attr(LIMIT)]
final class C extends Base implements I, J
{
    public function __construct(private readonly (A&B)|null $x = null) {}
    public function f(): void
    {
        $s = $this->db->select()->
            from('t');
        new Made(\strlen('x'), PHP_EOL);
        try {} catch (Gone) {} catch (One|Two $e) {}
        $a instanceof Kind;
        Stat::call(Konst::VALUE, Prop::$p, static::class);
        $f = function () use ($x) { return fn () => helper(); };
        echo "now() in a string", self::class;
        $x->method(); $x::method(); $x?->other();
    }
}
interface I extends K, L {}
enum E implements M {}
`
	refs := newFileSyntax(src, token.Latest).references()
	text := func(f func(int) string, n int) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, f(i))
		}
		return out
	}
	classes := text(func(i int) string {
		name := refs.classes[i].name.Bounds().Text(src)
		if refs.classes[i].attribute {
			return "#" + name
		}
		return name
	}, len(refs.classes))
	if want := []string{"#Attr", "Base", "I", "J", "Made", "Gone", "One", "Two", "Kind", "Stat", "Konst", "Prop", "static", "self", "K", "L", "M"}; !reflect.DeepEqual(classes, want) {
		t.Errorf("classes: %v", classes)
	}
	calls := text(func(i int) string { return refs.calls[i].Bounds().Text(src) }, len(refs.calls))
	if want := []string{`\strlen`, "helper"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("calls: %v", calls)
	}
	constants := text(func(i int) string { return refs.constants[i].Bounds().Text(src) }, len(refs.constants))
	if want := []string{"LIMIT", "null", "PHP_EOL"}; !reflect.DeepEqual(constants, want) {
		t.Errorf("constants: %v", constants)
	}
}

func TestUnknownClassInCatchWithoutVariable(t *testing.T) {
	src := "<?php\ntry {} catch (Missing) {}\n"
	findings := (&UnknownClassRule{}).Check(parser.ParseFile(src, token.Latest), src, symbols.NewIndex())
	if len(findings) != 1 || findings[0].Message != "Unknown class 'Missing'" || findings[0].StartLine != 1 || findings[0].StartCol != 14 || findings[0].EndCol != 21 {
		t.Errorf("got %+v", findings)
	}
}

func TestBuiltinUnavailableConstants(t *testing.T) {
	src := "<?php\nnamespace App;\n$a = \\JSON_THROW_ON_ERROR + JSON_THROW_ON_ERROR + Other\\JSON_THROW_ON_ERROR + PHP_EOL;\n"
	got := unavailableAndUnknown(t, src, "7.2")
	want := []string{
		"builtin-unavailable: Constant 'JSON_THROW_ON_ERROR' requires PHP >= 7.3",
		"builtin-unavailable: Constant 'JSON_THROW_ON_ERROR' requires PHP >= 7.3",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q", got)
	}
}

func TestSharedTreeIsParsedOncePerSource(t *testing.T) {
	shared := &Shared{}
	first := shared.tree("<?php f();", token.Latest)
	if shared.tree("<?php f();", token.Latest) != first || first.references() != first.references() {
		t.Error("the same source reuses its tree and references")
	}
	if shared.tree("<?php g();", token.Latest) == first {
		t.Error("a new source gets a new tree")
	}
	if shared.tree("<?php f();", token.PHP74) == first {
		t.Error("another PHP version gets a new tree")
	}
}

func TestMemberReferences(t *testing.T) {
	src := `<?php
isset($a->b->c[$d->e]); empty($f->g); unset($h->i); isset($z->y()->x);
$j->k ?? 1; $l->m ??= 2;
$n?->o(); P::Q; R::s(); $t->{$u}; $v::$w; $x->$y();
`
	var got []string
	for _, m := range newFileSyntax(src, token.Latest).references().members {
		desc := m.owner.Bounds().Text(src) + " " + m.name.Bounds().Text(src)
		if m.static {
			desc += " static"
		}
		if m.call {
			desc += " call"
		}
		if m.presence {
			desc += " presence"
		}
		got = append(got, desc)
	}
	want := []string{
		"$a b presence", "$a->b c presence", "$d e", "$f g presence", "$h i presence", "$z y call presence", "$z->y() x presence",
		"$j k presence", "$l m presence", "$n o call", "P Q static", "R s static call",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestUnknownMemberReadsAccessesFromTheTree(t *testing.T) {
	decl := `<?php
namespace App;
class Post { public function publish() {} }
`
	source := `<?php
namespace App;
class C {
    public function f(): void {
        if (isset($this->post->draft) || ($this->post->title ?? false)) {}
        $this->post
            ->missingOnNextLine();
        if ($this->other instanceof Unknown) { $this->other->gone(); }
        $this->post->nope();
    }
}
`
	got := unknownMemberMessages(checkUnknownMembers(t, map[string]string{"$this->post": `App\Post`, "$this->other": `App\Post`}, decl, source))
	if want := []string{"Unknown member 'nope' on 'Post'"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
