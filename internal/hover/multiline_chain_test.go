package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverMethodAfterMultilineArgumentList(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///v/EnumeratesValues.php", `<?php
namespace Illuminate\Support\Traits;
trait EnumeratesValues {
    /**
     * @template TMakeKey of array-key
     * @template TMakeValue
     * @param  \Illuminate\Contracts\Support\Arrayable<TMakeKey, TMakeValue>|iterable<TMakeKey, TMakeValue>|null  $items
     * @return static<TMakeKey, TMakeValue>
     */
    public static function make($items = []) { return new static($items); }
    /**
     * Execute a callback over each item.
     * @param  callable(TValue, TKey): mixed  $callback
     * @return $this
     */
    public function each(callable $callback) { return $this; }
}
`)
	idx.IndexFile("file:///v/Collection.php", `<?php
namespace Illuminate\Support;
use Illuminate\Support\Traits\EnumeratesValues;
/**
 * @template TKey of array-key
 * @template-covariant TValue
 */
class Collection {
    use EnumeratesValues;
}
`)
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	for name, src := range map[string]string{
		"oneline": `<?php
use Illuminate\Support\Collection;
class S { public function run(): void {
    Collection::make([1])->each(function ($d) {});
} }
`,
		"multiline": `<?php
use Illuminate\Support\Collection;
class S { public function run(): void {
    Collection::make([
        [
            'email' => 'a',
            'role' => UserRole::Admin->value,
        ],
    ])->each(function (array $data) {});
} }
`} {
		pos := charPosOf(t, src, "each", "->each")
		h := p.GetHover("file:///s.php", src, pos)
		if h == nil || !strings.Contains(h.Contents.Value, "function each") {
			t.Errorf("%s: no each hover: %+v", name, h)
		}
	}
}
