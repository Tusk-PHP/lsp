package parser

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func aliased(v reflect.Value, lo, hi uintptr, path string, seen map[uintptr]bool, found *[]string) {
	switch v.Kind() {
	case reflect.String:
		if s := v.String(); s != "" {
			if p := uintptr(unsafe.Pointer(unsafe.StringData(s))); p >= lo && p < hi {
				*found = append(*found, path)
			}
		}
	case reflect.Struct:
		for i := range v.NumField() {
			aliased(v.Field(i), lo, hi, path+"."+v.Type().Field(i).Name, seen, found)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			aliased(v.Index(i), lo, hi, path+"[]", seen, found)
		}
	case reflect.Pointer:
		if !v.IsNil() && !seen[v.Pointer()] {
			seen[v.Pointer()] = true
			aliased(v.Elem(), lo, hi, path, seen, found)
		}
	case reflect.Interface:
		if !v.IsNil() {
			aliased(v.Elem(), lo, hi, path, seen, found)
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			aliased(iter.Key(), lo, hi, path+"{key}", seen, found)
			aliased(iter.Value(), lo, hi, path+"{}", seen, found)
		}
	}
}

func TestDetachLeavesNoStringInTheSource(t *testing.T) {
	data, err := os.ReadFile("../../testdata/laravel/vendor/laravel/framework/src/Illuminate/Database/Eloquent/Model.php")
	if err != nil {
		t.Skip("the Laravel fixture's vendor tree isn't installed")
	}
	extra := `<?php
namespace App;
use Foo\{Bar, Baz as Qux};
#[Attr(name: 'x')]
/** @template T of object @property-read int $id @method static self make(string $a) */
final class Shop extends Base implements A, B {
    use T1, T2 { T1::run insteadof T2; }
    public const string NAME = 'shop';
    public private(set) int $total = 0 { get => $this->total; }
    public function __construct(private readonly ?Cart $cart = null, int ...$rest) {}
    /** @return array<string, int> */
    public function lines(): array { return new class { public $x = 1; }; }
}
interface I { public function f(int|string $a): static; }
trait T { abstract protected function g(); }
enum Size: string { case Small = 's'; const D = self::Small; }
function helper(callable $c = null): never {}
const TOP = 1;
`
	for _, source := range []string{string(data), extra} {
		file := ParseFile(source)
		file.Detach()
		lo := uintptr(unsafe.Pointer(unsafe.StringData(source)))
		var found []string
		aliased(reflect.ValueOf(file).Elem(), lo, lo+uintptr(len(source)), "FileNode", map[uintptr]bool{}, &found)
		if len(found) > 0 {
			t.Errorf("%d strings still point into the source, first at %s", len(found), found[0])
		}
	}
}

func TestDetachReachesEveryKindInTheFileNode(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type, string)
	walk = func(ty reflect.Type, path string) {
		if seen[ty] {
			return
		}
		seen[ty] = true
		switch ty.Kind() {
		case reflect.Map, reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
			t.Errorf("%s is a %s, which Detach doesn't copy strings out of", path, ty.Kind())
		case reflect.Struct:
			for i := range ty.NumField() {
				f := ty.Field(i)
				if !f.IsExported() && typeHoldsStrings(f.Type) {
					t.Errorf("%s.%s is unexported, so Detach can't set its strings", path, f.Name)
				}
				walk(f.Type, path+"."+f.Name)
			}
		case reflect.Slice, reflect.Array, reflect.Pointer:
			walk(ty.Elem(), path)
		}
	}
	walk(reflect.TypeOf(FileNode{}), "FileNode")
}

type detachSample struct {
	Count int
	Next  *detachSample
	Empty *detachSample
	Name  string
	Kids  []detachSample
}

func TestCollectStringsFollowsPointersOnce(t *testing.T) {
	root := &detachSample{Name: "root", Kids: []detachSample{{Name: "kid"}, {Count: 1}}}
	root.Next = &detachSample{Name: "next", Next: root}
	var got []string
	collectStrings(reflect.ValueOf(root).Elem(), map[uintptr]bool{uintptr(unsafe.Pointer(root)): true}, func(s reflect.Value) {
		got = append(got, s.String())
	})
	if strings.Join(got, ",") != "next,root,kid" {
		t.Errorf("got %v", got)
	}
	if !typeHoldsStrings(reflect.TypeOf(detachSample{})) || typeHoldsStrings(reflect.TypeOf([]int{})) {
		t.Error("a type holds strings when any field, element or pointee does")
	}
}
