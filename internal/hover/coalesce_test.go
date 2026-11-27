package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverCoalesceAndComparisonResults(t *testing.T) {
	source := `<?php
namespace App\Services;

use App\Models\Customer;
use App\Models\Chunk;

class Fields {
    /**
     * @param int      $userId
     * @param Customer $customer
     */
    public function save($userId, $customer = null) {
        $customer = $customer ?? new Customer($userId);
        $customer->save();
    }

    /** @param iterable<Chunk> $stream */
    public function stream(iterable $stream) {
        $usage = null;
        foreach ($stream as $chunk) {
            $usage = $chunk->getUsage() ?? $usage;
        }
        return $usage;
    }

    public function filter(array $criteria) {
        $hasProductCriteria = '' !== ($criteria['product'] ?? '');
        $negated = !($criteria['product'] ?? '');
        return [$hasProductCriteria, $negated];
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Models.php", `<?php
namespace App\Models;

class Customer {
    public function __construct(int $id) {}
    public function save(): void {}
}
interface TokenUsage {}
class Chunk {
    public function getUsage(): ?TokenUsage { return null; }
}
`)
	idx.IndexFile("file:///app/CategoryController.php", source)
	p := NewProvider(idx, nil, "laravel")
	for _, c := range []struct{ line, word, want string }{
		{"$customer->save();", "$customer", `Customer $customer`},
		{"$usage = $chunk->getUsage() ?? $usage;", "$usage", "TokenUsage"},
		{"$hasProductCriteria = ''", "$hasProductCriteria", "bool $hasProductCriteria"},
		{"$negated = !", "$negated", "bool $negated"},
	} {
		if got := hoverAt(t, p, source, c.line, c.word); !strings.Contains(got, c.want) {
			t.Errorf("hover on %s in %q: want %q in\n%s", c.word, c.line, c.want, got)
		}
	}
}
