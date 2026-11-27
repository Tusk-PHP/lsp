package completion

import (
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestCastAttributesCompleteAsWhatTheyAreCastTo(t *testing.T) {
	requireLaravelVendor(t)
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	indexLaravelVendor(idx, filepath.Join("..", "..", "testdata", "laravel"))
	dir := t.TempDir()
	index := func(name, src string) {
		writeProjectFile(t, dir, name, src)
		idx.IndexFile("file://"+filepath.Join(dir, name), src)
	}
	index("Order.php", `<?php
namespace App\Models;

use App\Casts\MoneyCast;
use App\OrderStatus;
use Illuminate\Database\Eloquent\Casts\AsCollection;
use Illuminate\Database\Eloquent\Casts\AsEnumCollection;
use Illuminate\Database\Eloquent\Model;

class Order extends Model
{
    protected $casts = ['status' => OrderStatus::class];

    protected function casts(): array
    {
        return [
            'total' => MoneyCast::class,
            'tags' => AsCollection::class,
            'statuses' => AsEnumCollection::of(OrderStatus::class),
        ];
    }
}
`)
	index("MoneyCast.php", `<?php
namespace App\Casts;

use App\Money;
use Illuminate\Contracts\Database\Eloquent\CastsAttributes;

class MoneyCast implements CastsAttributes
{
    public function get($model, string $key, mixed $value, array $attributes): Money { return new Money(); }
    public function set($model, string $key, mixed $value, array $attributes): string { return ''; }
}
`)
	index("Money.php", "<?php\nnamespace App;\nclass Money { public function inCents(): int { return 0; } }\n")
	index("OrderStatus.php", "<?php\nnamespace App;\nenum OrderStatus: string { case Paid = 'paid'; public function label(): string { return ''; } }\n")
	index("CastsAttributes.php", "<?php\nnamespace Illuminate\\Contracts\\Database\\Eloquent;\n/** @template TGet\n * @template TSet */\ninterface CastsAttributes {\n    /** @return TGet|null */\n    public function get($model, string $key, mixed $value, array $attributes);\n}\ninterface Castable {}\n")
	idx.SetCallForwards(laravel.CallForwards())
	models.AnalyzeEloquentModels(idx, dir)
	p := NewProvider(idx, nil, "laravel")

	for access, want := range map[string]string{
		"$order->status->":            "label",
		"$order->total->":             "inCents",
		"$order->tags->":              "pluck",
		"$order->statuses->first()->": "label",
	} {
		src := "<?php\nuse App\\Models\\Order;\nfunction f(Order $order) {\n    " + access + "|\n}\n"
		if labels := completeAtMarker(p, src); !labels[want] {
			t.Errorf("%s should offer %s(), got %v", access, want, mapKeys(labels, 20))
		}
	}
}
