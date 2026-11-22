package symbols

import "testing"

const overridesSource = `<?php
namespace App;

trait Greets {
    abstract public function greet(): string;
    public function wave(): string { return ''; }
}
class Base {
    use Greets;
    public const LIMIT = 1;
    private const SECRET = 1;
    public function greet(): string { return ''; }
    private function hidden(): void {}
}
class Child extends Base {
    public function greet(): string { return ''; }
}
class Orphan extends Missing {}
`

func TestOverridesInherited(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///o.php", overridesSource)
	for _, c := range []struct {
		class, method    string
		overrides, known bool
	}{
		{"App\\Child", "greet", true, true},   // Base's method
		{"App\\Child", "GREET", true, true},   // methods ignore case
		{"App\\Child", "wave", true, true},    // a parent's trait method
		{"App\\Child", "hidden", false, true}, // private isn't overridden
		{"App\\Child", "nothing", false, true},
		{"App\\Base", "greet", true, true}, // the class's own trait's abstract method
		{"App\\Base", "wave", false, true}, // the class's own trait's concrete method
		{"App\\Orphan", "greet", false, false},
	} {
		overrides, known := idx.OverridesInherited(c.class, c.method)
		if overrides != c.overrides || known != c.known {
			t.Errorf("OverridesInherited(%s, %s) = %v, %v; want %v, %v", c.class, c.method, overrides, known, c.overrides, c.known)
		}
	}
	for _, c := range []struct {
		name      string
		overrides bool
	}{{"LIMIT", true}, {"limit", false}, {"SECRET", false}, {"OTHER", false}} {
		if overrides, known := idx.OverridesInheritedConstant("App\\Child", c.name); overrides != c.overrides || !known {
			t.Errorf("OverridesInheritedConstant(Child, %s) = %v, %v", c.name, overrides, known)
		}
	}
}

// ClassMembersNamed answers from a per-class table that's rebuilt when the
// index changes.
func TestClassMembersNamedFollowsChanges(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///o.php", overridesSource)
	names := func(class, name string) int { return len(idx.ClassMembersNamed(class, name)) }
	if names("App\\Child", "greet") == 0 || names("App\\Child", "wave") == 0 || names("App\\Child", "nope") != 0 {
		t.Fatal("unexpected members of Child")
	}
	// Asked again, from the table.
	if names("App\\Child", "greet") == 0 {
		t.Fatal("greet lost on the second lookup")
	}
	// A change to the file drops the table.
	idx.IndexFile("file:///o.php", "<?php\nnamespace App;\nclass Child { public function only(): void {} }\n")
	if names("App\\Child", "greet") != 0 || names("App\\Child", "only") != 1 {
		t.Errorf("the table wasn't rebuilt after the change: greet=%d only=%d", names("App\\Child", "greet"), names("App\\Child", "only"))
	}
	// Many classes don't grow the cache past its limit.
	for i := 0; i < memberTableLimit+5; i++ {
		idx.ClassMembersNamed("App\\Child"+string(rune('A'+i%26))+string(rune('a'+i/26)), "x")
	}
	if n := len(idx.members.tables); n > memberTableLimit {
		t.Errorf("member tables grew to %d, past %d", n, memberTableLimit)
	}
}
