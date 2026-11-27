package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const castContracts = `<?php
namespace Illuminate\Contracts\Database\Eloquent;

/**
 * @template TGet
 * @template TSet
 */
interface CastsAttributes
{
    /** @return TGet|null */
    public function get($model, string $key, mixed $value, array $attributes);
}

interface Castable
{
    /** @return class-string<CastsAttributes>|CastsAttributes */
    public static function castUsing(array $arguments);
}
`

const castClasses = `<?php
namespace App;

use Illuminate\Contracts\Database\Eloquent\Castable;
use Illuminate\Contracts\Database\Eloquent\CastsAttributes;

class Money {}
class Euro extends Money {}
class Box {}

enum Status: string
{
    case Paid = 'paid';
}

/** @implements CastsAttributes<Money, Money> */
class MoneyCast implements CastsAttributes
{
    public function get($model, string $key, mixed $value, array $attributes) {}
}

class MixedCast implements CastsAttributes
{
    public function get($model, string $key, mixed $value, array $attributes): mixed {}
}

class AsAnything implements Castable
{
    /** @return CastsAttributes<mixed, mixed> */
    public static function castUsing(array $arguments) {}
}

class Plain {}

class InstanceCaster
{
    public function castUsing(array $arguments): MoneyCast {}
}

class Bare
{
    public $castUsing;
}

class AsMoney implements Castable
{
    /** @return CastsAttributes<Money, Money> */
    public static function castUsing(array $arguments) {}
}

class AsRaw implements Castable
{
    /** @return CastsAttributes */
    public static function castUsing(array $arguments) {}
}

class AsEither implements Castable
{
    /** @return MoneyCast|AsMoney */
    public static function castUsing(array $arguments) {}
}

class AsMoneyCast implements Castable
{
    public static function castUsing(array $arguments): MoneyCast {}
}

class Loop implements Castable
{
    public static function castUsing(array $arguments): Loop {}
}

class AsSecond implements Castable
{
    /**
     * @template T
     * @param array{mixed, class-string<T>} $arguments
     * @return CastsAttributes<T, T>
     */
    public static function castUsing(array $arguments) {}
}
`

func TestBuiltinCastTypeReadsEloquentsOwnCasts(t *testing.T) {
	for _, tc := range []struct {
		cast, want string
		ok         bool
	}{
		{"decimal:2", "string", true},
		{"immutable_datetime:Y-m-d", carbonImmutable, true},
		{" Boolean ", "bool", true},
		{"encrypted:collection", supportCollection, true},
		{`App\Casts\MoneyCast`, "", false},
	} {
		if got, ok := BuiltinCastType(tc.cast); got != tc.want || ok != tc.ok {
			t.Errorf("BuiltinCastType(%q) = %q, %v; want %q, %v", tc.cast, got, ok, tc.want, tc.ok)
		}
	}
}

func TestClassCastTypeIsWhatTheCastReads(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/contracts.php", castContracts)
	idx.IndexFile("file:///app/casts.php", castClasses)
	r := resolve.NewResolver(idx)
	for _, tc := range []struct {
		class string
		args  []string
		want  string
	}{
		{`App\Status`, nil, `App\Status`},
		{`App\MoneyCast`, nil, `?App\Money`},
		{`App\AsMoney`, nil, `App\Money`},
		{`App\AsMoney`, []string{`App\Euro`}, `App\Euro`},
		{`App\AsMoney`, []string{`App\Box`}, `App\Money`},
		{`App\AsMoneyCast`, nil, `?App\Money`},
		{`App\AsSecond`, []string{"", `App\Box`}, `App\Box`},
		{`App\MixedCast`, nil, "mixed"},
		{`App\AsAnything`, []string{`App\Box`}, "mixed"},
		{`App\AsRaw`, nil, "mixed"},
		{`App\AsEither`, nil, "mixed"},
		{`App\Loop`, nil, "mixed"},
		{`App\InstanceCaster`, nil, "mixed"},
		{`App\Bare`, nil, "mixed"},
		{`App\Plain`, nil, "mixed"},
		{`App\Missing`, nil, "mixed"},
	} {
		if got, _ := ClassCastType(r, tc.class, tc.args); got != tc.want {
			t.Errorf("ClassCastType(%s, %v) = %q, want %q", tc.class, tc.args, got, tc.want)
		}
	}
}

func TestClassCastTypeOfACastWhoseContractIsntIndexed(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/casts.php", `<?php
namespace App;

use Illuminate\Contracts\Database\Eloquent\CastsAttributes;

abstract class Partial implements CastsAttributes {}
`)
	if got, _ := ClassCastType(resolve.NewResolver(idx), `App\Partial`, nil); got != "mixed" {
		t.Errorf("a cast without get() reads as %q, want mixed", got)
	}
}
