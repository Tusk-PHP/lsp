package parser

import (
	"fmt"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"reflect"
	"strings"
	"testing"
)

func TestFileNodeProperties(t *testing.T) {
	file := ParseFile(`<?php
readonly class R {
    public function __construct(private int $a, $plain, public private(set) string $b = '' { get => $this->b; }) {}
}
class C {
    /** @var \Closure */
    private static \Closure $handler;
    var $legacy;
    protected int $x = 1, $y;
    public string $title { get => $this->title; SET => $value; }
    final public protected(set) int $z;
}
`, token.Latest)
	r, c := file.Classes[0], file.Classes[1]
	if got := propertySummary(r.Properties); got != "$a private  readonly | $b public private getreadonly" {
		t.Errorf("promoted: %s", got)
	}
	if got := propertySummary(c.Properties); got != "$handler private  static | $legacy public   | $x protected   | $y protected   | $title public  get,set | $z public protected " {
		t.Errorf("declared: %s", got)
	}
	if c.Properties[0].Type.Name != `\Closure` || c.Properties[0].DocComment != `/** @var \Closure */` {
		t.Errorf("typed property: %+v", c.Properties[0])
	}
	if p := r.Properties[0]; p.StartLine != 2 || p.StartCol != 44 {
		t.Errorf("promoted property position: %d:%d", p.StartLine, p.StartCol)
	}
}

func propertySummary(props []PropertyNode) string {
	var parts []string
	for _, p := range props {
		var kinds []string
		for _, h := range p.Hooks {
			kinds = append(kinds, h.Kind)
		}
		s := p.Name + " " + p.Visibility + " " + p.SetVisibility + " " + strings.Join(kinds, ",")
		if p.IsStatic {
			s += "static"
		}
		if p.IsReadonly {
			s += "readonly"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " | ")
}

func TestFileNodeMembers(t *testing.T) {
	file := ParseFile(`<?php
namespace App;
#[Attr(1, name: 'x')]
abstract class Base extends \Parent\Base implements A, B {
    /** uses */
    use T1, T2 { m as n; }
    use T3;
    const ONE = 1, TWO = [1, 2];
    protected const string LABEL = 'a' . 'b';
    abstract protected static function make(int|string $id = self::ONE, ?array &$out = null, Foo ...$rest): static;
    final public function new(/** The id. */ int $id, $tail /** Tail. */): void {}
}
`, token.Latest)
	c := file.Classes[0]
	if c.FullName != `App\Base` || c.Extends != `\Parent\Base` || !reflect.DeepEqual(c.Implements, []string{"A", "B"}) || !c.IsAbstract {
		t.Errorf("class header: %+v", c)
	}
	if !reflect.DeepEqual(c.Traits, []string{"T1", "T2", "T3"}) || c.TraitUseDoc != "/** uses */" {
		t.Errorf("traits: %v %q", c.Traits, c.TraitUseDoc)
	}
	if want := []AttributeDef{{Name: "Attr", Args: []string{"1", "name:'x'"}, Line: 2}}; !reflect.DeepEqual(c.Attributes, want) {
		t.Errorf("attributes: %+v", c.Attributes)
	}
	var consts []string
	for _, k := range c.Constants {
		consts = append(consts, k.Visibility+" "+k.Type.Name+" "+k.Name+"="+k.Value)
	}
	if got := strings.Join(consts, " | "); got != "public  ONE=1 | public  TWO=[1,2] | protected string LABEL='a'.'b'" {
		t.Errorf("constants: %s", got)
	}
	make, named := c.Methods[0], c.Methods[1]
	if make.Visibility != "protected" || !make.IsStatic || !make.IsAbstract || make.ReturnType.Name != "static" {
		t.Errorf("make: %+v", make)
	}
	if want := []ParamNode{
		{Name: "$id", Type: TypeNode{"int|string"}, DefaultValue: "self::ONE"},
		{Name: "$out", Type: TypeNode{"?array"}, DefaultValue: "null", IsReference: true},
		{Name: "$rest", Type: TypeNode{"Foo"}, IsVariadic: true},
	}; !reflect.DeepEqual(make.Params, want) {
		t.Errorf("params: %+v", make.Params)
	}
	if named.Name != "new" || !named.IsFinal || named.StartCol != 26 || named.Params[0].DocComment != "/** The id. */" || named.Params[1].DocComment != "/** Tail. */" {
		t.Errorf("new: %+v", named)
	}
}

func TestFileNodeDefaultValuesKeepTheirWords(t *testing.T) {
	file := ParseFile("<?php function f($a = new Foo ( 1 ), $b = 0o777, $c = \"x $y\" . 'z', $d = 1.5e3, $e = PHP_EOL /* c */, $f = <<<EOT\n  x\n  EOT) {}", token.Latest)
	var got []string
	for _, p := range file.Functions[0].Params {
		got = append(got, p.DefaultValue)
	}
	want := []string{"new Foo(1)", "0o777", "\"x $y\".'z'", "1.5e3", "PHP_EOL", "<<<EOT\n  x\n  EOT"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("defaults:\n%q\nwant\n%q", got, want)
	}
}

func TestFileNodeDeclarationsByScope(t *testing.T) {
	file := ParseFile(`<?php
namespace First;
use Foo\{A, function b};
const X = 1, Y = 2;
if (!class_exists('Poly')) {
    class Poly {}
    function poly() {}
}
function outer() {
    function inner() {}
    class Skipped {}
    $f = function () { function inClosure() {} };
}
class Host {
    public function m() { function inMethod() {} }
}
$fn = fn () => 1;
namespace Second;
interface I extends J, K { public function i(); public string $p { get; } const C = 1; }
trait T { use U; }
enum E: string implements I { use T; case A = 'a'; case B = 'b' . 'c'; const D = self::A; public function e() {} }
`, token.Latest)
	var functions []string
	for _, f := range file.Functions {
		functions = append(functions, f.FullName)
	}
	if want := []string{`First\poly`, `First\outer`, `First\inner`, `First\inClosure`}; !reflect.DeepEqual(functions, want) {
		t.Errorf("functions: %v", functions)
	}
	if len(file.Classes) != 2 || file.Classes[0].FullName != `First\Poly` || !file.Classes[0].Conditional || file.Classes[1].Conditional {
		t.Errorf("classes: %+v", file.Classes)
	}
	if len(file.Constants) != 2 || file.Constants[1].Name != "Y" || file.Constants[1].Value != "2" {
		t.Errorf("constants: %+v", file.Constants)
	}
	if len(file.Uses) != 2 || file.Uses[1] != (UseNode{FullName: `Foo\b`, Alias: "b", Kind: "function", StartLine: 2}) {
		t.Errorf("uses: %+v", file.Uses)
	}
	if file.Namespace != "Second" {
		t.Errorf("namespace: %q", file.Namespace)
	}
	i := file.Interfaces[0]
	if i.FullName != `Second\I` || !reflect.DeepEqual(i.Extends, []string{"J", "K"}) || len(i.Methods) != 1 || len(i.Properties) != 1 || len(i.Constants) != 1 {
		t.Errorf("interface: %+v", i)
	}
	if tr := file.Traits[0]; tr.FullName != `Second\T` || !reflect.DeepEqual(tr.Traits, []string{"U"}) {
		t.Errorf("trait: %+v", tr)
	}
	e := file.Enums[0]
	if e.BackedType != "string" || len(e.Cases) != 2 || e.Cases[1].Value != "'b'.'c'" || len(e.Constants) != 1 || len(e.Methods) != 1 || !reflect.DeepEqual(e.Traits, []string{"T"}) || !reflect.DeepEqual(e.Implements, []string{"I"}) {
		t.Errorf("enum: %+v", e)
	}
}

func TestFileNodeAnonymousClasses(t *testing.T) {
	src := `<?php
namespace N;
$a = new /* c */ readonly class(1) extends Base implements I, J
{
    public int $x;
    public function m() { return new class { const K = 1; }; }
};
$b = new #[A] class {};
`
	file := ParseFile(src, token.Latest)
	if len(file.AnonymousClasses) != 3 {
		t.Fatalf("got %+v", file.AnonymousClasses)
	}
	outer, inner, attributed := file.AnonymousClasses[0], file.AnonymousClasses[1], file.AnonymousClasses[2]
	if !strings.HasPrefix(outer.FullName, `N\class@anonymous@3@`) || outer.Name != AnonymousClassName || outer.Extends != "Base" || !reflect.DeepEqual(outer.Implements, []string{"I", "J"}) {
		t.Errorf("outer: %+v", outer)
	}
	if !outer.IsReadonly || !outer.Properties[0].IsReadonly || outer.StartCol != 26 || outer.BodyLine != 3 || outer.BodyCol != 0 || outer.EndLine != 6 {
		t.Errorf("outer layout: %+v", outer)
	}
	if inner.StartLine != 5 || len(inner.Constants) != 1 || inner.FullName == outer.FullName {
		t.Errorf("inner: %+v", inner)
	}
	if len(attributed.Attributes) != 1 || attributed.Attributes[0].Name != "A" {
		t.Errorf("attributed: %+v", attributed)
	}
	if file.AnonymousClassAt(4, 0) != &file.AnonymousClasses[0] {
		t.Error("the outer body holds line 4")
	}
}

func TestFileNodeAnonymousClassArgsAreInTheEnclosingScope(t *testing.T) {
	file := ParseFile("<?php new class(function () { function g() {} }) {};", token.Latest)
	if len(file.Functions) != 1 || file.Functions[0].Name != "g" {
		t.Errorf("functions: %+v", file.Functions)
	}
}

func TestFileNodeSkipsWhatHasNoName(t *testing.T) {
	for _, src := range []string{
		"<?php const = 1;",
		"<?php #[] function f() {}",
		"<?php function f(int , $b) {}",
		"<?php use ;",
		"<?php class K { public $a { ; } }",
		"<?php class K { const = 2; }",
		"<?php enum E { case; }",
		"<?php class K { public int ; }",
	} {
		file := ParseFile(src, token.Latest)
		if len(file.Errors) == 0 {
			t.Errorf("%q: expected syntax errors", src)
		}
		summary := fmt.Sprintf("%+v", *file)
		if strings.Contains(summary, "Name: ") {
			t.Errorf("%q: a nameless entry was kept: %s", src, summary)
		}
	}
}

func TestFileNodeReadsAroundComments(t *testing.T) {
	file := ParseFile("<?php function f(int /* a */ | # b\n string $x = 'it\\'s' /* c */, $y = \"open\n", token.Latest)
	params := file.Functions[0].Params
	if params[0].Type.Name != "int|string" || params[0].DefaultValue != `'it\'s'` || params[1].DefaultValue != "\"open\n" {
		t.Errorf("params: %+v", params)
	}
	for src, want := range map[string][2]int{
		"<?php new class // c\n{};":     {1, 0},
		"<?php new class /* c */ {};":   {0, 24},
		"<?php new class /* never ends": {0, 29},
		"<?php new class // c":          {0, 20},
		"<?php new class \n":            {1, 0},
		"<?php new class(1) {};":        {0, 19},
	} {
		file := ParseFile(src, token.Latest)
		if got := [2]int{file.AnonymousClasses[0].BodyLine, file.AnonymousClasses[0].BodyCol}; got != want {
			t.Errorf("%q: body at %v, want %v", src, got, want)
		}
	}
}

func TestFileNodeSetVisibility(t *testing.T) {
	file := ParseFile("<?php class C { public function __construct(public(set) int $a, protected(set) int $b) {} }", token.Latest)
	if got := propertySummary(file.Classes[0].Properties); got != "$a public public  | $b public protected " {
		t.Errorf("got %s", got)
	}
}

func TestFileNodeErrorPositions(t *testing.T) {
	file := ParseFile("<?php\n$a = ;\n/* open", token.Latest)
	want := []ParseError{
		{Message: "syntax error, unexpected ';', expected an expression", Line: 1, Column: 5},
		{Message: "unterminated comment", Line: 2, Column: 0},
	}
	if !reflect.DeepEqual(file.Errors, want) {
		t.Errorf("errors: %+v", file.Errors)
	}
}

func TestParseFileReadsTheGrammarItIsGiven(t *testing.T) {
	src := "<?php\n#[ a comment before PHP 8\nfunction match() {}\n"
	old, current := ParseFile(src, token.PHP74), ParseFile(src, token.PHP80)
	if old.Grammar != token.PHP74 || len(old.Errors) != 0 || len(old.Functions) != 1 {
		t.Errorf("PHP 7.4: grammar %s, errors %v, functions %v", old.Grammar, old.Errors, old.Functions)
	}
	if current.Grammar != token.PHP80 || len(current.Errors) == 0 {
		t.Errorf("PHP 8.0: grammar %s, errors %v", current.Grammar, current.Errors)
	}
}
