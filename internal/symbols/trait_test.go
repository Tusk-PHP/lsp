package symbols

import (
	"testing"
)

func TestTraitMembersResolvedOnClass(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///trait.php", `<?php
namespace App;

trait Loggable {
	public string $logPrefix = '';

	public function log(string $message): void {}
}
`)
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class UserService {
	use Loggable;

	public function doWork(): void {}
}
`)

	members := idx.GetClassMembers(`App\UserService`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}

	if !found["doWork"] {
		t.Error("expected own method doWork")
	}
	if !found["log"] {
		t.Error("expected trait method log")
	}
	if !found["$logPrefix"] {
		t.Error("expected trait property $logPrefix")
	}
}

func TestNestedTraitMembers(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///base_trait.php", `<?php
namespace App;

trait HasTimestamps {
	public function touchTimestamp(): void {}
	public string $updatedAt = '';
}
`)
	idx.IndexFile("file:///middle_trait.php", `<?php
namespace App;

trait Auditable {
	use HasTimestamps;

	public function getAuditLog(): array {}
}
`)
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class Order {
	use Auditable;

	public function total(): float {}
}
`)

	members := idx.GetClassMembers(`App\Order`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}

	if !found["total"] {
		t.Error("expected own method total")
	}
	if !found["getAuditLog"] {
		t.Error("expected method from Auditable trait")
	}
	if !found["touchTimestamp"] {
		t.Error("expected method from nested HasTimestamps trait")
	}
	if !found["$updatedAt"] {
		t.Error("expected property from nested HasTimestamps trait")
	}
}

func TestDeeplyNestedTraitMembers(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", `<?php
namespace App;

trait TraitA {
	public function fromA(): void {}
}
`)
	idx.IndexFile("file:///b.php", `<?php
namespace App;

trait TraitB {
	use TraitA;
	public function fromB(): void {}
}
`)
	idx.IndexFile("file:///c.php", `<?php
namespace App;

trait TraitC {
	use TraitB;
	public function fromC(): void {}
}
`)
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class MyClass {
	use TraitC;
	public function own(): void {}
}
`)

	members := idx.GetClassMembers(`App\MyClass`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}

	for _, name := range []string{"own", "fromC", "fromB", "fromA"} {
		if !found[name] {
			t.Errorf("expected method %s from nested trait chain", name)
		}
	}
}

func TestMultipleTraitsOnClass(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///t1.php", `<?php
namespace App;

trait Searchable {
	public function search(string $query): array {}
}
`)
	idx.IndexFile("file:///t2.php", `<?php
namespace App;

trait Cacheable {
	public function cache(int $ttl): void {}
}
`)
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class ProductRepository {
	use Searchable, Cacheable;

	public function findById(int $id): ?Product {}
}
`)

	members := idx.GetClassMembers(`App\ProductRepository`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}

	for _, name := range []string{"findById", "search", "cache"} {
		if !found[name] {
			t.Errorf("expected method %s", name)
		}
	}
}

func TestTraitWithInheritance(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///trait.php", `<?php
namespace App;

trait SoftDeletes {
	public function restore(): void {}
}
`)
	idx.IndexFile("file:///base.php", `<?php
namespace App;

class Model {
	public function save(): bool {}
}
`)
	idx.IndexFile("file:///child.php", `<?php
namespace App;

class User extends Model {
	use SoftDeletes;

	public function fullName(): string {}
}
`)

	members := idx.GetClassMembers(`App\User`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}

	for _, name := range []string{"fullName", "restore", "save"} {
		if !found[name] {
			t.Errorf("expected method %s from trait+inheritance chain", name)
		}
	}
}

func TestTraitMapCleanupOnReindex(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///trait.php", `<?php
namespace App;

trait OldTrait {
	public function oldMethod(): void {}
}

trait NewTrait {
	public function newMethod(): void {}
}
`)

	// First index: class uses OldTrait
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class Foo {
	use OldTrait;
}
`)

	members := idx.GetClassMembers(`App\Foo`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}
	if !found["oldMethod"] {
		t.Fatal("expected oldMethod before re-index")
	}

	// Re-index: class now uses NewTrait instead
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class Foo {
	use NewTrait;
}
`)

	members = idx.GetClassMembers(`App\Foo`)
	found = map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}
	if found["oldMethod"] {
		t.Error("oldMethod should not appear after re-index removed OldTrait")
	}
	if !found["newMethod"] {
		t.Error("expected newMethod after re-index added NewTrait")
	}
}

func traitRulesIndex() *Index {
	idx := NewIndex()
	idx.IndexFile("file:///traits.php", `<?php
namespace App;
interface Speaks { public function bark(): void; }
trait A { public function talk(): void {} public function shout(): void {} public $volume; }
trait B { public function talk(): void {} public function shout(): void {} public function whisper(): void {} }
trait Nested { use B; }
class Talker implements Speaks {
    use B, A {
        A::talk insteadof B;
        B::shout insteadof A;
        B::talk as bTalk;
        A::shout as protected bark;
        whisper as private;
    }
}
class NestedTalker { use Nested, A { A::talk insteadof Nested; } }
class NestedAlias { use Nested { Nested::whisper as murmur; talk as chat; } }
class Overrides { use A { talk as speak; } public function speak(): void {} }
enum Mood { use A, B { B::talk insteadof A; B::shout insteadof A; } }
`)
	return idx
}

func TestTraitInsteadofPicksTheMethodPHPUses(t *testing.T) {
	idx := traitRulesIndex()
	cases := []struct{ class, member, want string }{
		{`App\Talker`, "talk", `App\A`},
		{`App\Talker`, "shout", `App\B`},
		{`App\NestedTalker`, "talk", `App\A`},
		{`App\Mood`, "talk", `App\B`},
	}
	for _, c := range cases {
		named := idx.ClassMembersNamed(c.class, c.member)
		if len(named) != 1 || named[0].ParentFQN != c.want {
			var got []string
			for _, m := range named {
				got = append(got, m.ParentFQN)
			}
			t.Errorf("%s::%s comes from %v, want only %s", c.class, c.member, got, c.want)
		}
	}
	if named := idx.ClassMembersNamed(`App\Talker`, "volume"); len(named) != 1 {
		t.Errorf("a property is untouched by method rules: %d", len(named))
	}
}

func TestTraitAsAliasesAMethod(t *testing.T) {
	idx := traitRulesIndex()
	cases := []struct{ class, member, wantTrait, wantName, wantVisibility string }{
		{`App\Talker`, "bTalk", `App\B`, "talk", "public"},
		{`App\Talker`, "bark", `App\A`, "shout", "protected"},
		{`App\Talker`, "whisper", `App\B`, "whisper", "private"},
		{`App\NestedAlias`, "murmur", `App\B`, "whisper", "public"},
		{`App\NestedAlias`, "chat", `App\B`, "talk", "public"},
	}
	for _, c := range cases {
		named := idx.ClassMembersNamed(c.class, c.member)
		if len(named) == 0 {
			t.Errorf("%s::%s isn't found", c.class, c.member)
			continue
		}
		m := named[0]
		original := idx.Lookup(c.wantTrait + "::" + c.wantName)
		if traitMethodOf(m).FQN != original.FQN || m.Visibility != c.wantVisibility || m.Range != original.Range {
			t.Errorf("%s::%s = %s (%s, %v), want %s::%s (%s)", c.class, c.member, traitMethodOf(m).FQN, m.Visibility, m.Range, c.wantTrait, c.wantName, c.wantVisibility)
		}
		if c.member != c.wantName && (m.FQN != c.class+"::"+c.member || m.ParentFQN != c.class) {
			t.Errorf("%s::%s has the identity %s in %s, want its own", c.class, c.member, m.FQN, m.ParentFQN)
		}
		if original.Visibility != "public" {
			t.Errorf("aliasing changed %s::%s itself: %s", c.wantTrait, c.wantName, original.Visibility)
		}
	}
	if named := idx.ClassMembersNamed(`App\Overrides`, "speak"); len(named) == 0 || named[0].ParentFQN != `App\Overrides` {
		t.Errorf("the class's own method wins over an alias: %v", named)
	}
}

func TestTraitRulesLeaveWithTheirFile(t *testing.T) {
	idx := traitRulesIndex()
	idx.IndexFile("file:///traits.php", `<?php
namespace App;
trait A { public function talk(): void {} }
trait B { public function talk(): void {} }
class Talker { use B, A; }
`)
	if named := idx.ClassMembersNamed(`App\Talker`, "bTalk"); len(named) != 0 {
		t.Errorf("an alias outlives its rule: %v", named)
	}
	if named := idx.ClassMembersNamed(`App\Talker`, "talk"); len(named) != 2 || named[0].ParentFQN != `App\B` {
		t.Errorf("without insteadof both traits' talk are listed in use order: %v", named)
	}
}

func TestEachDeclarationHasItsOwnTraitRules(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///traits.php", "<?php\ntrait A { public function talk() {} }\ntrait B { public function talk() {} }\n")
	idx.IndexFile("file:///a.php", "<?php\nclass Dup { use A, B { A::talk insteadof B; B::talk as bTalk; } }\n")
	idx.IndexFile("file:///b.php", "<?php\nclass Dup { use A, B { B::talk insteadof A; A::talk as aTalk; } }\n")

	origin := func(members []*Symbol, name string) string {
		for _, m := range members {
			if m.Name == name {
				return traitMethodOf(m).ParentFQN
			}
		}
		return ""
	}
	for _, def := range idx.Definitions("Dup") {
		members := idx.DefinitionMembers(def)
		want := map[string][3]string{"file:///a.php": {"A", "bTalk", "B"}, "file:///b.php": {"B", "aTalk", "A"}}[def.URI]
		if got := origin(members, "talk"); got != want[0] {
			t.Errorf("%s: talk comes from %q, want %s", def.URI, got, want[0])
		}
		if got := origin(members, want[1]); got != want[2] {
			t.Errorf("%s: alias %s comes from %q, want %s", def.URI, want[1], got, want[2])
		}
	}

	idx.IndexFile("file:///b.php", "<?php\n")
	if named := idx.ClassMembersNamed("Dup", "bTalk"); len(named) != 1 || traitMethodOf(named[0]).ParentFQN != "B" {
		t.Errorf("the promoted declaration brings back its own rules: %v", named)
	}
	if named := idx.ClassMembersNamed("Dup", "aTalk"); len(named) != 0 {
		t.Errorf("the removed declaration's alias leaves with it: %v", named)
	}
}

func traitMethodOf(member *Symbol) *Symbol {
	if member.AliasOf != nil {
		return member.AliasOf
	}
	return member
}
