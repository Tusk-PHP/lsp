package models

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestEloquentAttributeAccessorTypes(t *testing.T) {
	const uri = "file:///app/Models/Order.php"
	const source = `<?php
namespace App\Models;

use App\Money;
use Carbon\Carbon;
use Illuminate\Database\Eloquent\Casts\Attribute;
use Illuminate\Database\Eloquent\Model;

class Order extends Model
{
    /** @return Attribute<int, never> */
    protected function count(): Attribute { return Attribute::make(get: fn () => 1); }

    /** @return \Illuminate\Database\Eloquent\Casts\Attribute<Money, Money> */
    protected function total(): Attribute { return Attribute::make(get: fn () => new Money()); }

    /** @return Attribute<?Discount, never> */
    protected function discount(): Attribute { return Attribute::make(get: fn () => null); }

    /** @return Attribute<\Illuminate\Support\Collection<int, Money>, never> */
    protected function payments(): Attribute { return Attribute::make(get: fn () => collect()); }

    /** @return Attribute */
    protected function title(): Attribute { return Attribute::make(get: fn (): string => ''); }

    protected function code(): Attribute { return Attribute::make(fn ($value): int => 1); }

    protected function paidAt(): Attribute
    {
        return new Attribute(get: function ($value): ?Carbon { return null; });
    }

    protected function shippedAt(): Attribute { return Attribute::get(fn (): Carbon => now()); }

    protected function secret(): Attribute { return Attribute::set(fn (string $value): string => $value); }

    protected function plain(): Attribute { return Attribute::make(get: fn ($value) => $value); }

    /** @return string */
    protected function label(): Attribute { return Attribute::make(get: fn () => ''); }

    abstract protected function nickname(): Attribute;

    protected function rating(): Attribute
    {
        $rating = Attribute::make(get: fn (): float => 1.0);
        return $rating;
    }

    protected function helper(): Attribute { return Helper::make(fn (): int => 1); }

    protected function flag(): Attribute { return Attribute::make(set: fn ($value): string => $value, get: fn (): bool => true); }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", reanalyzeBaseModel)
	idx.IndexFile(uri, source)
	AnalyzeModelFile(idx, "/", "laravel", uri, source, nil, nil)
	want := map[string]string{
		"count":     "int",
		"total":     `App\Money`,
		"discount":  `?App\Models\Discount`,
		"payments":  `Illuminate\Support\Collection<int, App\Money>`,
		"title":     "string",
		"code":      "int",
		"paidAt":    `?Carbon\Carbon`,
		"shippedAt": `Carbon\Carbon`,
		"secret":    "mixed",
		"plain":     "mixed",
		"label":     "string",
		"nickname":  "mixed",
		"rating":    "float",
		"helper":    "mixed",
		"flag":      "bool",
	}
	for name, typ := range want {
		sym := idx.Lookup(`App\Models\Order::$` + name)
		if sym == nil {
			t.Errorf("Order has no $%s", name)
			continue
		}
		if sym.Type != typ {
			t.Errorf("$%s: got %q, want %q", name, sym.Type, typ)
		}
	}
}
