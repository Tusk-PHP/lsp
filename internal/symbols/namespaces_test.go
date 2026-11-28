package symbols

import "testing"

func TestIndexDeclarationsUnderTheirOwnNamespace(t *testing.T) {
	sources := map[string]string{
		"statements": `<?php
namespace A;
use X\Dep;
use function X\helper;
class Foo extends Dep {}
interface IA {}
trait TA {}
enum EA {}
function fa(Dep $d): Dep {}
const CA = 1;
$a = new class extends Dep {};
namespace\fa();
namespace B;
class Bar extends Dep {}
interface IB {}
trait TB {}
enum EB {}
function fb(Dep $d): Dep {}
const CB = 1;
$b = new class extends Dep {};
`,
		"braced": `<?php
namespace A {
use X\Dep;
class Foo extends Dep {}
interface IA {}
trait TA {}
enum EA {}
function fa(Dep $d): Dep {}
const CA = 1;
$a = new class extends Dep {};
}
namespace B {
class Bar extends Dep {}
interface IB {}
trait TB {}
enum EB {}
function fb(Dep $d): Dep {}
const CB = 1;
$b = new class extends Dep {};
}
`,
		"one line": `<?php namespace A; use X\Dep; class Foo extends Dep {} interface IA {} trait TA {} enum EA {} function fa(Dep $d): Dep {} const CA = 1; $a = new class extends Dep {}; namespace B; class Bar extends Dep {} interface IB {} trait TB {} enum EB {} function fb(Dep $d): Dep {} const CB = 1; $b = new class extends Dep {};`,
	}
	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			idx := NewIndex()
			idx.IndexFile("file:///multi.php", source)
			want := map[string]SymbolKind{
				`A\Foo`: KindClass, `A\IA`: KindInterface, `A\TA`: KindTrait, `A\EA`: KindEnum, `A\fa`: KindFunction, `A\CA`: KindConstant,
				`B\Bar`: KindClass, `B\IB`: KindInterface, `B\TB`: KindTrait, `B\EB`: KindEnum, `B\fb`: KindFunction, `B\CB`: KindConstant,
			}
			for fqn, kind := range want {
				sym := idx.Lookup(fqn)
				if sym == nil || sym.Kind != kind {
					t.Errorf("Lookup(%q) = %+v, want a symbol of kind %v", fqn, sym, kind)
				}
			}
			for _, fqn := range []string{`B\Foo`, `B\fa`, `B\CA`, `A\Bar`, `Foo`} {
				if sym := idx.Lookup(fqn); sym != nil {
					t.Errorf("Lookup(%q) = %+v, want nothing: it is declared in another namespace", fqn, sym)
				}
			}
			for fqn, extends := range map[string]string{`A\Foo`: `X\Dep`, `B\Bar`: `B\Dep`} {
				if sym := idx.Lookup(fqn); sym == nil || sym.Extends != extends {
					t.Errorf("%s extends %+v, want %q: imports hold only in their own namespace", fqn, sym, extends)
				}
			}
			for fqn, typ := range map[string]string{`A\fa`: `X\Dep`, `B\fb`: `B\Dep`} {
				sym := idx.Lookup(fqn)
				if sym == nil || sym.ReturnType != typ || len(sym.Params) != 1 || sym.Params[0].Type != typ {
					t.Errorf("%s = %+v, want parameter and return type %q", fqn, sym, typ)
				}
			}
			anonymous := map[string]string{}
			for _, sym := range idx.fileSymbols["file:///multi.php"] {
				if sym.Kind == KindClass && sym.Name == "class@anonymous" {
					anonymous[namespaceForFQN(sym.FQN)] = sym.Extends
				}
			}
			if anonymous["A"] != `X\Dep` || anonymous["B"] != `B\Dep` || len(anonymous) != 2 {
				t.Errorf("anonymous classes by namespace = %v, want A extending X\\Dep and B extending B\\Dep", anonymous)
			}
		})
	}
}

func TestIndexGlobalNamespaceBlock(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///multi.php", `<?php
namespace A {
class Foo {}
}
namespace {
use X\Dep;
class G extends Dep {}
function fg() {}
const CG = 1;
}
`)
	for fqn, kind := range map[string]SymbolKind{`A\Foo`: KindClass, "G": KindClass, "fg": KindFunction, "CG": KindConstant} {
		if sym := idx.Lookup(fqn); sym == nil || sym.Kind != kind {
			t.Errorf("Lookup(%q) = %+v, want a symbol of kind %v", fqn, sym, kind)
		}
	}
	if sym := idx.Lookup("G"); sym == nil || sym.Extends != `X\Dep` {
		t.Errorf("G = %+v, want it to extend X\\Dep through the global block's import", sym)
	}
}

func TestIDEHelperFileWithSeveralNamespaces(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///app/Models/User.php", "<?php\nnamespace App\\Models;\nclass User {}\n")
	idx.IndexIDEHelperFile("file:///_ide_helper_models.php", `<?php
namespace App\Models {
/**
 * @property int $id
 */
class User {}
}
namespace App\Billing {
use Money\Money;
/**
 * @property Money $total
 */
class Invoice {
    public Money $amount;
}
}
`)
	if sym := idx.Lookup(`App\Models\User::$id`); sym == nil || !sym.IsVirtual {
		t.Errorf("App\\Models\\User::$id = %+v, want the helper's virtual property", sym)
	}
	if sym := idx.Lookup(`App\Billing\Invoice::$total`); sym == nil || sym.Type != `Money\Money` {
		t.Errorf("App\\Billing\\Invoice::$total = %+v, want type Money\\Money", sym)
	}
	if sym := idx.Lookup(`App\Billing\Invoice::$amount`); sym == nil || sym.Type != `Money\Money` {
		t.Errorf("App\\Billing\\Invoice::$amount = %+v, want type Money\\Money", sym)
	}
	if sym := idx.Lookup(`App\Billing\User`); sym != nil {
		t.Errorf("App\\Billing\\User = %+v, want nothing: User is in App\\Models", sym)
	}
}
