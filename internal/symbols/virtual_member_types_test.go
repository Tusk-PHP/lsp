package symbols

import "testing"

func TestVirtualMemberTypesResolveOnlyClassNames(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///app/Post.php", `<?php
namespace App;
use Lib\Tag;
/**
 * @template TValue
 * @property array{id: int, tags: list<string>} $meta
 * @property array<string, Item> $map
 * @property list<Item> $list
 * @property non-empty-array<int, Item> $nonEmpty
 * @property int<0, max> $count
 * @property Box<Item> $box
 * @property ?Box<Item> $maybeBox
 * @property Item|Box<Item>|null $either
 * @property Item&Countable $both
 * @property Box<int|Item>|null $nested
 * @property Item[] $items
 * @property Tag $tag
 * @property \Lib\Other $other
 * @property TValue $value
 * @property ?TValue $maybeValue
 * @property 'draft'|'published' $state
 * @property 1|2 $level
 * @property callable(Item): void $callback
 * @property  Item | null  $spaced
 * @method array{ok: bool} result()
 * @method Box<Item> box()
 */
class Post {}
`)
	idx.IndexIDEHelperFile("file:///_ide_helper.php", `<?php
namespace App;
/**
 * @property array{a: int} $helperMeta
 * @property Item $helperItem
 */
class Post {}
`)
	for name, want := range map[string]string{
		"$meta":       "array{id: int, tags: list<string>}",
		"$map":        "array<string, Item>",
		"$list":       "list<Item>",
		"$nonEmpty":   "non-empty-array<int, Item>",
		"$count":      "int<0, max>",
		"$box":        `App\Box<Item>`,
		"$maybeBox":   `?App\Box<Item>`,
		"$either":     `App\Item|App\Box<Item>|null`,
		"$both":       `App\Item&App\Countable`,
		"$nested":     `App\Box<int|Item>|null`,
		"$items":      `App\Item[]`,
		"$tag":        `Lib\Tag`,
		"$other":      `Lib\Other`,
		"$value":      "TValue",
		"$maybeValue": "?TValue",
		"$state":      "'draft'|'published'",
		"$level":      "1|2",
		"$callback":   "callable(Item): void",
		"$spaced":     `App\Item|null`,
		"$helperMeta": "array{a: int}",
		"$helperItem": `App\Item`,
	} {
		sym := idx.Lookup(`App\Post::` + name)
		if sym == nil {
			t.Errorf("%s is not indexed", name)
			continue
		}
		if sym.Type != want {
			t.Errorf("%s: type %q, want %q", name, sym.Type, want)
		}
	}
	for name, want := range map[string]string{
		"result": "array{ok: bool}",
		"box":    `App\Box<Item>`,
	} {
		if sym := idx.Lookup(`App\Post::` + name); sym == nil || sym.ReturnType != want {
			t.Errorf("%s: got %+v, want return type %q", name, sym, want)
		}
	}
}
