package models

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

var castStubs = []string{
	`<?php
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
`,
	`<?php
namespace Illuminate\Database\Eloquent\Casts;

use Illuminate\Contracts\Database\Eloquent\Castable;

class AsStringable implements Castable
{
    /** @return \Illuminate\Contracts\Database\Eloquent\CastsAttributes<\Illuminate\Support\Stringable, string|\Stringable> */
    public static function castUsing(array $arguments) {}
}

class AsCollection implements Castable
{
    /** @return \Illuminate\Contracts\Database\Eloquent\CastsAttributes<\Illuminate\Support\Collection<array-key, mixed>, iterable> */
    public static function castUsing(array $arguments) {}

    /** @return string */
    public static function using($class, $map = null) {}
}

class AsEnumCollection implements Castable
{
    /**
     * @template TEnum of \UnitEnum
     *
     * @param  array{class-string<TEnum>}  $arguments
     * @return \Illuminate\Contracts\Database\Eloquent\CastsAttributes<\Illuminate\Support\Collection<array-key, TEnum>, iterable<TEnum>>
     */
    public static function castUsing(array $arguments) {}

    /** @return string */
    public static function of($class) {}
}
`,
	`<?php
namespace Illuminate\Support;

class Collection {}
class Stringable {}
`,
	`<?php
namespace App\Support;

class OptionCollection extends \Illuminate\Support\Collection {}
`,
	`<?php
namespace App\Casts;

use App\Money;
use Illuminate\Contracts\Database\Eloquent\Castable;
use Illuminate\Contracts\Database\Eloquent\CastsAttributes;

/** @implements CastsAttributes<Money, Money> */
class MoneyCast implements CastsAttributes
{
    public function get($model, string $key, mixed $value, array $attributes) {}
}

class AddressCast implements CastsAttributes
{
    public function get($model, string $key, mixed $value, array $attributes): \App\Address {}
}

class Location implements Castable
{
    public static function castUsing(array $arguments): AddressCast {}
}

class EitherCast implements CastsAttributes
{
    public function get($model, string $key, mixed $value, array $attributes): \App\Money|\App\Address {}
}

class BothCast implements CastsAttributes
{
    public function get($model, string $key, mixed $value, array $attributes): \App\Money&\App\Address {}
}
`,
	`<?php
namespace App;

class Money {}
class Address {}

enum OrderStatus: string
{
    case Paid = 'paid';
}
`,
}

func castModel(body string) string {
	return `<?php
namespace App\Models;

use App\Casts\Location;
use App\Casts\MoneyCast;
use App\OrderStatus;
use App\Support\OptionCollection;
use Illuminate\Database\Eloquent\Casts\AsCollection;
use Illuminate\Database\Eloquent\Casts\AsEnumCollection;
use Illuminate\Database\Eloquent\Casts\AsStringable;
use Illuminate\Database\Eloquent\Casts\Attribute;
use Illuminate\Database\Eloquent\Model;

class Order extends Model
{
` + body + `
}
`
}

const orderURI = "file:///app/Models/Order.php"

func analyzeOrder(t *testing.T, source string, schema *Schema, migrations map[string]map[string]*MigrationColumn) *symbols.Index {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", reanalyzeBaseModel)
	for i, stub := range castStubs {
		idx.IndexFile("file:///vendor/casts"+strconv.Itoa(i)+".php", stub)
	}
	idx.IndexFile(orderURI, source)
	AnalyzeModelFile(idx, "/", "laravel", orderURI, source, schema, migrations)
	return idx
}

func propertyType(t *testing.T, idx *symbols.Index, name string) string {
	t.Helper()
	sym := idx.Lookup(`App\Models\Order::$` + name)
	if sym == nil {
		t.Fatalf("Order has no $%s", name)
	}
	return sym.Type
}

func castForms(entries string) map[string]string {
	return map[string]string{
		"property": castModel("    protected $casts = [\n" + entries + "\n    ];"),
		"method":   castModel("    protected function casts(): array\n    {\n        return [\n" + entries + "\n        ];\n    }"),
	}
}

func TestEloquentBuiltinCasts(t *testing.T) {
	casts := []struct{ attribute, cast, want string }{
		{"a_int", "'int'", "int"},
		{"a_integer", "'integer'", "int"},
		{"a_real", "'real'", "float"},
		{"a_float", "'float'", "float"},
		{"a_double", "'double'", "float"},
		{"a_decimal", "'decimal:2'", "string"},
		{"a_string", "'string'", "string"},
		{"a_bool", "'bool'", "bool"},
		{"a_boolean", "'Boolean'", "bool"},
		{"a_object", "'object'", "stdClass"},
		{"a_array", "'array'", "array"},
		{"a_json", "'json'", "array"},
		{"a_json_unicode", "'json:unicode'", "array"},
		{"a_collection", "'collection'", `Illuminate\Support\Collection`},
		{"a_date", "'date'", `Illuminate\Support\Carbon`},
		{"a_datetime", "'datetime'", `Illuminate\Support\Carbon`},
		{"a_custom_datetime", "'datetime:Y-m-d'", `Illuminate\Support\Carbon`},
		{"a_custom_date", `"date:Y-m-d"`, `Illuminate\Support\Carbon`},
		{"a_immutable_date", "'immutable_date'", `Carbon\CarbonImmutable`},
		{"a_immutable_datetime", "'immutable_datetime'", `Carbon\CarbonImmutable`},
		{"a_immutable_custom", "'immutable_datetime:Y-m-d H:i'", `Carbon\CarbonImmutable`},
		{"a_timestamp", "'timestamp'", "int"},
		{"a_hashed", "'hashed'", "string"},
		{"a_encrypted", "'encrypted'", "string"},
		{"a_encrypted_array", "'encrypted:array'", "array"},
		{"a_encrypted_collection", "'encrypted:collection'", `Illuminate\Support\Collection`},
		{"a_encrypted_json", "'encrypted:json'", "array"},
		{"a_encrypted_object", "'encrypted:object'", "stdClass"},
	}
	var entries strings.Builder
	for _, c := range casts {
		entries.WriteString("        '" + c.attribute + "' => " + c.cast + ",\n")
	}
	for form, source := range castForms(entries.String()) {
		idx := analyzeOrder(t, source, nil, nil)
		for _, c := range casts {
			if got := propertyType(t, idx, c.attribute); got != c.want {
				t.Errorf("%s: %s cast to %s: got %q, want %q", form, c.attribute, c.cast, got, c.want)
			}
		}
	}
}

func TestEloquentClassCasts(t *testing.T) {
	casts := []struct{ attribute, cast, want string }{
		{"status", "OrderStatus::class", `App\OrderStatus`},
		{"total", "MoneyCast::class", `?App\Money`},
		{"total_string", `'App\Casts\MoneyCast'`, `?App\Money`},
		{"total_with_arguments", `MoneyCast::class.':EUR'`, `?App\Money`},
		{"address", "Location::class", `App\Address`},
		{"title", "AsStringable::class", `Illuminate\Support\Stringable`},
		{"tags", "AsCollection::class", `Illuminate\Support\Collection<array-key, mixed>`},
		{"options", "AsCollection::using(OptionCollection::class)", `App\Support\OptionCollection`},
		{"statuses", "AsEnumCollection::of(OrderStatus::class)", `Illuminate\Support\Collection<array-key, App\OrderStatus>`},
		{"statuses_concatenated", "AsEnumCollection::class.':'.OrderStatus::class", `Illuminate\Support\Collection<array-key, App\OrderStatus>`},
		{"unknown", `\App\Casts\Missing::class`, "mixed"},
	}
	var entries strings.Builder
	for _, c := range casts {
		entries.WriteString("        '" + c.attribute + "' => " + c.cast + ",\n")
	}
	for form, source := range castForms(entries.String()) {
		idx := analyzeOrder(t, source, nil, nil)
		for _, c := range casts {
			if got := propertyType(t, idx, c.attribute); got != c.want {
				t.Errorf("%s: %s cast to %s: got %q, want %q", form, c.attribute, c.cast, got, c.want)
			}
		}
	}
}

func TestEloquentUnknownCastStringKeepsTheColumn(t *testing.T) {
	source := castModel(`    protected $casts = ['note' => 'no_such_cast'];`)
	migrations := map[string]map[string]*MigrationColumn{"orders": {"note": {Name: "note", Type: "string"}}}
	idx := analyzeOrder(t, source, nil, migrations)
	if got := propertyType(t, idx, "note"); got != "string" {
		t.Errorf("got %q, want the column's string", got)
	}
}

func TestEloquentCastPrecedence(t *testing.T) {
	source := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;

/**
 * @property string $documented
 */
class Order extends Model
{
    protected $casts = [
        'paid_at' => 'datetime',
        'shipped_at' => 'datetime',
        'overridden' => 'string',
        'documented' => 'array',
        'accessed' => 'array',
        'twice' => 'string',
        'twice' => 'int',
        'not_null' => 'datetime',
    ];

    protected function casts(): array
    {
        return ['overridden' => 'boolean'];
    }

    public function getAccessedAttribute(): int
    {
        return 1;
    }
}
`
	schema := &Schema{Source: SchemaSourceLive, Tables: []SchemaTable{{
		Name: "orders",
		Columns: []SchemaColumn{
			{Name: "paid_at", DataType: "varchar", ColumnType: "varchar(255)", IsNullable: true},
			{Name: "not_null", DataType: "timestamp", ColumnType: "timestamp"},
		},
	}}}
	migrations := map[string]map[string]*MigrationColumn{"orders": {
		"shipped_at": {Name: "shipped_at", Type: "string", Nullable: true},
	}}
	idx := analyzeOrder(t, source, schema, migrations)
	want := map[string]string{
		"paid_at":    `?Illuminate\Support\Carbon`,
		"shipped_at": `?Illuminate\Support\Carbon`,
		"not_null":   `Illuminate\Support\Carbon`,
		"overridden": "bool",
		"documented": "string",
		"accessed":   "int",
		"twice":      "int",
	}
	for name, typ := range want {
		if got := propertyType(t, idx, name); got != typ {
			t.Errorf("$%s: got %q, want %q", name, got, typ)
		}
	}
}

func TestEloquentCastPointsAtItsEntry(t *testing.T) {
	source := castModel(`    protected $casts = ['paid_at' => 'datetime'];`)
	idx := analyzeOrder(t, source, nil, nil)
	sym := idx.Lookup(`App\Models\Order::$paid_at`)
	if sym == nil {
		t.Fatal("Order has no $paid_at")
	}
	line := strings.Split(source, "\n")[sym.Range.Start.Line]
	if got := line[sym.Range.Start.Character:sym.Range.End.Character]; got != "'paid_at'" {
		t.Errorf("range covers %q, want the 'paid_at' key", got)
	}
	if sym.DocComment != "Cast: 'datetime'" {
		t.Errorf("doc: got %q", sym.DocComment)
	}
}

func TestEloquentCastsItCantReadLeaveTheColumn(t *testing.T) {
	entries := []string{
		"0 => 'int'",
		"'' => 'int'",
		"'missing_class' => 'App\\Missing'",
		"'dynamic' => $caster::using(OptionCollection::class)",
		"'joined' => 'App' . 'Casts'",
		"'suffixed' => MoneyCast::class . $suffix",
		"'plain_suffix' => MoneyCast::class . 'EUR'",
		"'created' => new MoneyCast()",
	}
	columns := map[string]*MigrationColumn{}
	for _, name := range []string{"missing_class", "dynamic", "joined", "suffixed", "plain_suffix", "created"} {
		columns[name] = &MigrationColumn{Name: name, Type: "string"}
	}
	for form, source := range castForms("        " + strings.Join(entries, ",\n        ") + ",") {
		idx := analyzeOrder(t, source, nil, map[string]map[string]*MigrationColumn{"orders": columns})
		for name := range columns {
			if got := propertyType(t, idx, name); got != "string" {
				t.Errorf("%s: $%s: got %q, want the column's string", form, name, got)
			}
		}
		if idx.Lookup(`App\Models\Order::$`) != nil || idx.Lookup(`App\Models\Order::$0`) != nil {
			t.Errorf("%s: a cast without a name made a property", form)
		}
	}
}

func TestEloquentCastsReadOnlyTheArrayCastsReturns(t *testing.T) {
	source := castModel(`    protected function casts(): array
    {
        $later = function () {
            return ['inner' => 'int'];
        };
        return ['outer' => 'bool'];
    }`)
	idx := analyzeOrder(t, source, nil, nil)
	if got := propertyType(t, idx, "outer"); got != "bool" {
		t.Errorf("$outer: got %q, want bool", got)
	}
	if idx.Lookup(`App\Models\Order::$inner`) != nil {
		t.Error("an array a closure in casts() returns was read as casts")
	}
}

func TestEloquentNullableColumnsMakeTheirCastsNullable(t *testing.T) {
	source := castModel(`    protected $casts = [
        'total' => MoneyCast::class,
        'either' => \App\Casts\EitherCast::class,
        'both' => \App\Casts\BothCast::class,
    ];`)
	migrations := map[string]map[string]*MigrationColumn{"orders": {
		"total":  {Name: "total", Type: "string", Nullable: true},
		"either": {Name: "either", Type: "string", Nullable: true},
		"both":   {Name: "both", Type: "string", Nullable: true},
	}}
	idx := analyzeOrder(t, source, nil, migrations)
	for name, want := range map[string]string{
		"total":  `?App\Money`,
		"either": `App\Money|App\Address|null`,
		"both":   `(App\Money&App\Address)|null`,
	} {
		if got := propertyType(t, idx, name); got != want {
			t.Errorf("$%s: got %q, want %q", name, got, want)
		}
	}
}

func TestEloquentCastsFollowTheirClassAcrossAnalyses(t *testing.T) {
	source := castModel(`    protected $casts = ['total' => MoneyCast::class];`)
	idx := analyzeOrder(t, source, nil, nil)
	if got := propertyType(t, idx, "total"); got != `?App\Money` {
		t.Fatalf("$total before the cast class changes: %q", got)
	}
	idx.IndexFile("file:///vendor/casts4.php", strings.Replace(castStubs[4], "CastsAttributes<Money, Money>", `CastsAttributes<\App\Address, \App\Address>`, 1))
	dependsOn := AnalyzeModelFile(idx, "/", "laravel", orderURI, source, nil, nil)
	if got := propertyType(t, idx, "total"); got != `?App\Address` {
		t.Errorf("$total after the cast class changed: %q, want ?App\\Address", got)
	}
	if !slices.Contains(dependsOn, `App\Casts\MoneyCast`) {
		t.Errorf("the analysis depends on %v, want MoneyCast among them", dependsOn)
	}
}

func TestAnalyzeEloquentModelsReportsWhatEachModelFilesCastsDependOn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Order.php")
	source := castModel(`    protected $casts = ['total' => MoneyCast::class, 'paid_at' => 'datetime'];`)
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", reanalyzeBaseModel)
	for i, stub := range castStubs {
		idx.IndexFile("file:///vendor/casts"+strconv.Itoa(i)+".php", stub)
	}
	uri := "file://" + path
	idx.IndexFile(uri, source)
	dependencies := AnalyzeEloquentModels(idx, dir)
	if !slices.Contains(dependencies[uri], `App\Casts\MoneyCast`) {
		t.Errorf("Order.php's casts depend on %v, want MoneyCast among them", dependencies[uri])
	}
	if _, ok := dependencies["file:///vendor/Model.php"]; ok {
		t.Error("a file without models has cast dependencies")
	}
}
