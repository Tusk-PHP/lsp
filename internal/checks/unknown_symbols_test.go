package checks

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// TestAttributeWithArgsNoFalsePositives reproduces the user-reported bug where
// attribute classes used with constructor arguments were flagged as unknown
// functions, and named arguments inside them (e.g. `nullable: true`) were
// flagged as unknown attributes.
func TestAttributeWithArgsNoFalsePositives(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.3"})
	idx.IndexFile("file:///attrs.php", `<?php
namespace Toramanlis\ImplicitMigrations\Attributes;
#[\Attribute]
class Char {}
#[\Attribute]
class Unique {}
`)

	source := `<?php
namespace App\Models;

use Toramanlis\ImplicitMigrations\Attributes\Char;
use Toramanlis\ImplicitMigrations\Attributes\Unique;

#[Unique(["ext_id", "email"])]
#[Char("picture", nullable: true)]
#[Char("name", nullable: false)]
#[Char("ext_id", nullable: true)]
class User
{
}
`
	file := parser.ParseFile(source)

	fnFindings := (&UnknownFunctionRule{}).Check(file, source, idx)
	for _, f := range fnFindings {
		t.Errorf("unexpected unknown-function finding: %q at line %d col %d", f.Message, f.StartLine, f.StartCol)
	}

	clsFindings := (&UnknownClassRule{}).Check(file, source, idx)
	for _, f := range clsFindings {
		t.Errorf("unexpected unknown-class/attribute finding: %q at line %d col %d", f.Message, f.StartLine, f.StartCol)
	}
}

// TestUnknownClassUnknownAttribute verifies that an attribute with an unknown
// class-like name is flagged by UnknownClassRule.
func TestUnknownClassUnknownAttribute(t *testing.T) {
	source := `<?php
class Foo {
    #[NeverHeardOfThisAttribute]
    public function bar(): void {}
}
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.3"})

	rule := &UnknownClassRule{}
	findings := rule.Check(file, source, idx)

	var relevant []Finding
	for _, f := range findings {
		if f.Code == "unknown-class" {
			relevant = append(relevant, f)
		}
	}
	if len(relevant) == 0 {
		t.Fatal("expected at least one unknown-class finding for NeverHeardOfThisAttribute, got none")
	}
	found := false
	for _, f := range relevant {
		if strings.Contains(f.Message, "NeverHeardOfThisAttribute") && strings.Contains(strings.ToLower(f.Message), "attribute") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a finding mentioning 'NeverHeardOfThisAttribute' and 'attribute', got: %v", relevant)
	}
}

// TestUnknownClassIgnoresPropertyBeforeStaticAccess checks that in
// `$this->type::tryFrom($v)` the property holding a class name isn't read as
// a class.
func TestUnknownClassIgnoresPropertyBeforeStaticAccess(t *testing.T) {
	source := `<?php
namespace App;

class Holder {
    public function go($value, $useFactory, $a, $k) {
        $value = $this->type::tryFrom($value);
        $related = $this->related::class;
        $macro = $this->grammar::hasMacro($value);
        $factory = $useFactory->factoryClass::new();
        $soft = $this->throughParent::isSoftDeletable();
        $deep = $a->b->c::d();
        $safe = $this?->type::X;
        $spaced = $this -> type :: X;
        $made = self::$map[$k]::make();
        $name = $value::class;
        $split = $this->
            type::tryFrom($value);
        return Missing::bar();
    }
}
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.3"})

	var got []string
	for _, f := range (&UnknownClassRule{}).Check(file, source, idx) {
		got = append(got, f.Message)
	}
	if strings.Join(got, ",") != "Unknown class 'Missing'" {
		t.Errorf("only Missing should be reported, got %v", got)
	}
	for _, f := range (&UnknownFunctionRule{}).Check(file, source, idx) {
		t.Errorf("unexpected unknown-function finding: %q at line %d", f.Message, f.StartLine)
	}
}

func TestUnknownFunctionRuleSkipsKeywords(t *testing.T) {
	source := `<?php
class B {
    public function a($p, $x)
    {
        $f = function ($t) use ($x) { return $t; };
        $g = fn ($v) => $v;
        $h = (1 and (2)) or (3) xor (4);
        list($c) = $p;
        switch ($c) { case (1): break; }
        $y = $c instanceof ($p);
        $p($c);
        static($c);
        yield ($c);
        declare(strict_types=1);
        throw ($c);
        return (1);
    }
    public function b()
    {
        return function ($joinTable) {
            return $joinTable;
        };
    }
}
`
	if findings := (&UnknownFunctionRule{}).Check(parser.ParseFile(source), source, symbols.NewIndex()); len(findings) != 0 {
		t.Errorf("unexpected findings: %+v", findings)
	}

	source = "<?php\nmissing_function(1);\n"
	findings := (&UnknownFunctionRule{}).Check(parser.ParseFile(source), source, symbols.NewIndex())
	if len(findings) != 1 {
		t.Errorf("want missing_function reported, got %+v", findings)
	}
}

// checkUnknownMembers indexes the fixtures, then runs the rule over the last one
// with $vars typed as the resolver answers, in fully qualified names.
func checkUnknownMembers(t *testing.T, vars map[string]string, sources ...string) []Finding {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for i, source := range sources {
		src := symbols.SourceProject
		if strings.Contains(source, "namespace Vendor") {
			src = symbols.SourceVendor
		}
		idx.IndexFileWithSource("file:///f"+strconv.Itoa(i)+".php", source, src)
	}
	source := sources[len(sources)-1]
	rule := &UnknownMemberRule{TypeResolver: func(expr, _ string, _ int, _ *parser.FileNode) string {
		return vars[expr]
	}}
	return rule.Check(parser.ParseFile(source), source, idx)
}

func unknownMemberMessages(findings []Finding) []string {
	messages := []string{}
	for _, f := range findings {
		messages = append(messages, f.Message)
	}
	return messages
}

func TestUnknownMemberResolvedTypes(t *testing.T) {
	post := `<?php
namespace App;
class Post { public string $title = ''; public function publish(): void {} }
`
	thread := `<?php
namespace App;
class Thread { public function lock(): void {} }
`
	cases := []struct {
		name string
		typ  string
		use  string
		want []string
	}{
		{"fully qualified, member exists", `App\Post`, `$v->publish();`, nil},
		{"fully qualified, member missing", `App\Post`, `$v->missing();`, []string{"Unknown member 'missing' on 'Post'"}},
		{"leading backslash", `\App\Post`, `$v->missing();`, []string{"Unknown member 'missing' on 'Post'"}},
		{"nullable", `?App\Post`, `$v?->missing();`, []string{"Unknown member 'missing' on 'Post'"}},
		{"or null", `App\Post|null`, `$v->missing();`, []string{"Unknown member 'missing' on 'Post'"}},
		{"generic", `App\Post<int, App\Thread>`, `$v->missing();`, []string{"Unknown member 'missing' on 'Post'"}},
		{"union, on the first", `App\Post|App\Thread`, `$v->publish();`, nil},
		{"union, on the second", `App\Post|App\Thread`, `$v->lock();`, nil},
		{"union, on neither", `App\Post|App\Thread`, `$v->missing();`, []string{"Unknown member 'missing' on 'Post|Thread'"}},
		{"union with generics", `App\Post<int, string>|App\Thread`, `$v->lock();`, nil},
		{"union with a scalar", `App\Post|string`, `$v->missing();`, []string{"Unknown member 'missing' on 'Post'"}},
		{"union with an unknown class", `App\Post|App\Gone`, `$v->missing();`, nil},
		{"union with mixed", `App\Post|mixed`, `$v->missing();`, nil},
		{"intersection, on the second", `App\Post&App\Thread`, `$v->lock();`, nil},
		{"unknown class", `App\Gone`, `$v->missing();`, nil},
		{"a relative answer from a call", `static`, `$v->missing();`, nil},
		{"method names ignore case", `App\Post`, `$v->PUBLISH();`, nil},
		{"property names don't", `App\Post`, `$v->TITLE;`, []string{"Unknown member 'TITLE' on 'Post'"}},
		{"chain continued from an earlier line", `App\Post`, `})->missing();`, nil},
		{"class name", `App\Post`, `$v::class;`, nil},
		{"tested with instanceof", `App\Post`, "if ($v instanceof \\App\\Thread) {\n$v->lock();\n}", nil},
		{"tested with instanceof, on neither", `App\Post`, "if ($v instanceof \\App\\Thread) {\n$v->missing();\n}", []string{"Unknown member 'missing' on 'Post|Thread'"}},
		{"tested against an unknown class", `App\Post`, "if ($v instanceof \\App\\Gone) {\n$v->missing();\n}", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := "<?php\nnamespace App\\Http;\n" + tc.use + "\n"
			got := unknownMemberMessages(checkUnknownMembers(t, map[string]string{"$v": tc.typ}, post, thread, source))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("type %q: got %v, want %v", tc.typ, got, tc.want)
			}
		})
	}
}

func TestUnknownMemberOwnersThatCanAnswerForMore(t *testing.T) {
	cases := []struct {
		name  string
		owner string
		decl  string
		use   string
		want  []string
	}{
		{"__call answers calls", `App\Owner`, `class Owner { public function __call($n, $a) {} }`, `$v->anything();`, nil},
		{"__call doesn't answer properties", `App\Owner`, `class Owner { public function __call($n, $a) {} }`, `$v->anything;`, []string{"Unknown member 'anything' on 'Owner'"}},
		{"__get answers properties", `App\Owner`, `class Owner { public function __get($n) {} }`, `$v->anything;`, nil},
		{"__get doesn't answer calls", `App\Owner`, `class Owner { public function __get($n) {} }`, `$v->anything();`, []string{"Unknown member 'anything' on 'Owner'"}},
		{"inherited __callStatic", `App\Owner`, `class Base { public static function __callStatic($n, $a) {} } class Owner extends Base {}`, `\App\Owner::anything();`, nil},
		{"@mixin", `App\Owner`, "/** @mixin \\App\\Other */\nclass Owner {}", `$v->anything();`, nil},
		{"@method tag", `App\Owner`, "/** @method void tagged() */\nclass Owner {}", `$v->tagged();`, nil},
		{"unknown parent", `App\Owner`, `class Owner extends Gone {}`, `$v->anything();`, nil},
		{"known parent", `App\Owner`, `class Base { public function up() {} } class Owner extends Base {}`, `$v->up(); $v->anything();`, []string{"Unknown member 'anything' on 'Owner'"}},
		{"promoted property", `App\Owner`, `class Owner { public function __construct(private int $id) {} }`, `$v->id;`, nil},
		{"member of a parent interface", `App\Owner`, `interface Up { function up(); } interface Owner extends Up {}`, `$v->up(); $v->anything();`, []string{"Unknown member 'anything' on 'Owner'"}},
		{"abstract class relying on its interface", `App\Owner`, `interface Up { function up(); } abstract class Owner implements Up {}`, `$v->up();`, nil},
		{"trait", `App\Owner`, `trait Owner {}`, `$v->anything();`, nil},
		{"member from a trait", `App\Owner`, `trait Up { public function up() {} } class Owner { use Up; }`, `$v->up(); $v->anything();`, []string{"Unknown member 'anything' on 'Owner'"}},
		{"enum basics", `App\Owner`, `enum Owner: string { case A = 'a'; }`, `$v->name; $v->value; $v::from('a'); $v::tryFrom('a'); $v::cases(); $v->anything;`, []string{"Unknown member 'anything' on 'Owner'"}},
		{"pure enum has no value", `App\Owner`, `enum Owner { case A; }`, `$v->name; $v->value;`, []string{"Unknown member 'value' on 'Owner'"}},
		{"enum method", `App\Owner`, `enum Owner { case A; public function up() {} }`, `$v->up();`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decl := "<?php\nnamespace App;\n" + tc.decl + "\n"
			source := "<?php\nnamespace App\\Http;\n" + tc.use + "\n"
			got := unknownMemberMessages(checkUnknownMembers(t, map[string]string{"$v": tc.owner}, decl, source))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUnknownMemberInterfaces(t *testing.T) {
	vendor := "<?php\nnamespace Vendor;\ninterface Contract { function up(); }\n"
	source := "<?php\nnamespace App;\ninterface Own { function up(); }\nclass Impl implements \\Vendor\\Contract { function up() {} }\n$v->anything();\n"

	if got := checkUnknownMembers(t, map[string]string{"$v": `Vendor\Contract`}, vendor, source); len(got) != 0 {
		t.Errorf("a vendor's interface can't show a member is missing, got %v", unknownMemberMessages(got))
	}
	if got := checkUnknownMembers(t, map[string]string{"$v": `App\Own`}, vendor, source); len(got) != 1 {
		t.Errorf("the project's own interface should be checked, got %v", unknownMemberMessages(got))
	}
	if got := checkUnknownMembers(t, map[string]string{"$v": `App\Impl`}, vendor, source); len(got) != 1 {
		t.Errorf("a class should be checked even if it implements a vendor's interface, got %v", unknownMemberMessages(got))
	}
}

func TestUnknownMemberBuiltinWithoutMembersIsNotFlagged(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	source := "<?php\nnamespace App;\n$v->open();\n"
	rule := &UnknownMemberRule{TypeResolver: func(string, string, int, *parser.FileNode) string { return "ZipArchive" }}
	if sym := idx.Lookup("ZipArchive"); sym == nil || len(sym.Children) != 0 {
		t.Skip("ZipArchive now has its members listed")
	}
	if got := rule.Check(parser.ParseFile(source), source, idx); len(got) != 0 {
		t.Errorf("got %v", unknownMemberMessages(got))
	}
}

// TestUnknownFunctionRuleNewerSyntax covers syntax that looks like a call to
// the line-based rule but isn't: by-reference declarations, `new (expr)`,
// `yield from`, an anonymous class whose `new` is on the previous line, PHP 8.4
// asymmetric visibility and property hooks, and DNF property types.
func TestUnknownFunctionRuleNewerSyntax(t *testing.T) {
	source := `<?php
class C
{
    public private(set) ?string $attribute = null;
    protected(set) string $name = '';
    private (\Countable&\Traversable)|null $list = null;

    public array $map {
        set (array $value) { $this->map = $value; }
        get => $this->map;
    }
    public string $short {
        set (string $value) => $value . "";
    }

    public function &get(string $name): array { return $this->map; }
    public function &__get($name): mixed { return $this->map[$name]; }

    public function make(object $data): iterable
    {
        $copy = new ($data::class)();
        $other = (new ($this->name()))->run();
        yield from $this->map;
        yield from (new static())->make($copy);
        return new
            class($other) {};
    }
}
function &byRef(array &$a) { return $a; }
`
	if findings := (&UnknownFunctionRule{}).Check(parser.ParseFile(source), source, symbols.NewIndex()); len(findings) != 0 {
		t.Errorf("unexpected findings: %+v", findings)
	}

	// The names are still checked when they are real calls.
	source = "<?php\n$a = set(1);\n$b = from(2) + get(3);\n$c = yielded_from(4);\n"
	findings := (&UnknownFunctionRule{}).Check(parser.ParseFile(source), source, symbols.NewIndex())
	var names []string
	for _, f := range findings {
		names = append(names, f.Message)
	}
	want := []string{"Unknown function 'set'", "Unknown function 'from'", "Unknown function 'get'", "Unknown function 'yielded_from'"}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Errorf("got %v, want %v", names, want)
	}
}

// TestUnknownMemberBuiltinClassesHaveAllMembers checks that builtin classes
// with declared members list all of them, so inherited and version-specific
// members such as ReflectionClass::getAttributes() or
// DateTime::createFromTimestamp() aren't reported, and that a member newer
// than the project's PHP version still is.
func TestUnknownMemberBuiltinClassesHaveAllMembers(t *testing.T) {
	source := `<?php
$class->getParentClass();
$class->getFileName();
$class->getProperties();
$class->getAttributes();
$class->name;
$object->getProperty('x');
$param->getType();
$param->isVariadic();
$prop->getAttributes();
$method->getAttributes();
$named->getName();
$e->getFile();
$e->getLine();
$e->getPrevious();
$invalid->getTraceAsString();
$date->modify('+1 day');
$x = ReflectionProperty::IS_PUBLIC;
$d = DateTime::createFromFormat('Y', '2024');
$t = DateTime::createFromTimestamp(1);
`
	vars := map[string]string{
		"$class": "ReflectionClass", "$object": "ReflectionObject", "$param": "ReflectionParameter",
		"$prop": "ReflectionProperty", "$method": "ReflectionMethod", "$named": "ReflectionNamedType",
		"$e": "Exception", "$invalid": "InvalidArgumentException", "$date": "DateTime",
	}
	check := func(version string) []string {
		idx := symbols.NewIndex()
		idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: version})
		rule := &UnknownMemberRule{TypeResolver: func(expr, _ string, _ int, _ *parser.FileNode) string {
			return vars[expr]
		}}
		return unknownMemberMessages(rule.Check(parser.ParseFile(source), source, idx))
	}
	if got := check("8.4"); len(got) != 0 {
		t.Errorf("PHP 8.4: got %v", got)
	}
	got := check("8.3")
	if len(got) != 1 || !strings.Contains(got[0], "createFromTimestamp") {
		t.Errorf("PHP 8.3: want only createFromTimestamp reported, got %v", got)
	}
}

func TestUnknownFunctionWindowsBuiltins(t *testing.T) {
	source := "<?php\nsapi_windows_vt100_support(STDOUT, true);\nsapi_windows_set_ctrl_handler(null);\nsapi_windows_cp_get();\n"
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	file := parser.ParseFile(source)
	if got := (&UnknownFunctionRule{}).Check(file, source, idx); len(got) != 0 {
		t.Errorf("got %v", unknownMemberMessages(got))
	}
}
