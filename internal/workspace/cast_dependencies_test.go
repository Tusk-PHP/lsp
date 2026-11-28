package workspace

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const castContract = `<?php
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
`

func moneyCast(returns string) string {
	return `<?php
namespace App\Casts;

use Illuminate\Contracts\Database\Eloquent\CastsAttributes;

class MoneyCast implements CastsAttributes
{
    public function get($model, string $key, mixed $value, array $attributes): ` + returns + ` {}
}
`
}

const castOrder = `<?php
namespace App\Models;

use App\Casts\MoneyCast;
use Illuminate\Database\Eloquent\Model;

class Order extends Model
{
    protected $casts = ['total' => MoneyCast::class, 'paid_at' => 'datetime'];
}
`

type castWorkspace struct {
	ws                *Bootstrapped
	root              string
	castURI, orderURI string
}

func newCastWorkspace(t testing.TB) castWorkspace {
	t.Helper()
	root := t.TempDir()
	migration := "<?php\nSchema::create('orders', function (Blueprint $table) {\n    $table->string('total')->nullable();\n});\n"
	files := map[string]string{
		"database/migrations/2024_01_01_000000_create_orders_table.php": migration,
		"app/Models/Order.php":    castOrder,
		"app/Casts/MoneyCast.php": moneyCast(`\App\Money`),
	}
	for name, src := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.DefaultConfig()
	off := false
	cfg.DatabaseEnabled = &off
	ws := New(Options{RootPath: root, Framework: "laravel", Config: cfg, Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	ws.Index.IndexFile("file:///vendor/Model.php", "<?php\nnamespace Illuminate\\Database\\Eloquent;\nabstract class Model {}\n")
	ws.Index.IndexFile("file:///vendor/CastsAttributes.php", castContract)
	cw := castWorkspace{
		ws:       ws,
		root:     root,
		castURI:  symbols.PathToURI(filepath.Join(root, "app", "Casts", "MoneyCast.php")),
		orderURI: symbols.PathToURI(filepath.Join(root, "app", "Models", "Order.php")),
	}
	ws.Index.IndexFile(cw.castURI, files["app/Casts/MoneyCast.php"])
	ws.Index.IndexFile(cw.orderURI, castOrder)
	ws.AnalyzeModels()
	return cw
}

func (cw castWorkspace) total(t *testing.T) string {
	t.Helper()
	sym := cw.ws.Index.Lookup(`App\Models\Order::$total`)
	if sym == nil {
		t.Fatal("Order has no $total")
	}
	return sym.Type
}

func TestEditingACastClassRefreshesTheModelsThatUseIt(t *testing.T) {
	cw := newCastWorkspace(t)
	if got := cw.total(t); got != `?App\Money` {
		t.Fatalf("before the edit: got %q", got)
	}

	dependents := cw.ws.IndexFile(cw.castURI, moneyCast(`\App\Price`))
	if !slices.Equal(dependents, []string{cw.orderURI}) {
		t.Fatalf("editing the cast class should name Order's file, got %q", dependents)
	}
	if got := cw.total(t); got != `?App\Money` {
		t.Fatalf("the model was refreshed before RefreshModels: got %q", got)
	}
	cw.ws.RefreshModels(dependents)
	if got := cw.total(t); got != `?App\Price` {
		t.Errorf("after the edit: got %q, want ?App\\Price", got)
	}

	if dependents := cw.ws.RemoveFile(cw.castURI); !slices.Equal(dependents, []string{cw.orderURI}) {
		t.Fatalf("removing the cast class should name Order's file, got %q", dependents)
	}
	cw.ws.RefreshModels([]string{cw.orderURI})
	if got := cw.total(t); got != "mixed" {
		t.Errorf("after removing the cast class: got %q, want mixed", got)
	}
}

func TestEditingACastClassOnDiskRefreshesTheModels(t *testing.T) {
	cw := newCastWorkspace(t)
	path := filepath.Join(cw.root, "app", "Casts", "MoneyCast.php")
	if err := os.WriteFile(path, []byte(moneyCast(`\App\Price`)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := cw.ws.Refresh(Refresh{Paths: []string{path}}); err != nil {
		t.Fatal(err)
	}
	if got := cw.total(t); got != `?App\Price` {
		t.Errorf("got %q, want ?App\\Price", got)
	}
}

func TestCastDependentsFollowTheModelFile(t *testing.T) {
	cw := newCastWorkspace(t)
	if dependents := cw.ws.IndexFile(cw.orderURI, castOrder+"\n"); len(dependents) != 0 {
		t.Errorf("a model's own edit names no other file, got %q", dependents)
	}
	withoutCast := strings.Replace(castOrder, "'total' => MoneyCast::class, ", "", 1)
	cw.ws.IndexFile(cw.orderURI, withoutCast)
	if dependents := cw.ws.IndexFile(cw.castURI, moneyCast(`\App\Price`)); len(dependents) != 0 {
		t.Errorf("Order no longer casts with MoneyCast, got %q", dependents)
	}
	if cw.ws.castDependents.tracking() {
		t.Errorf("no cast class is used any more, yet %d are tracked", len(cw.ws.castDependents.byClass))
	}
}

func TestCastDependentsMatchAClassHoweverItIsWritten(t *testing.T) {
	var d castDependents
	d.set("file:///Order.php", []string{`App\Casts\MoneyCast`, `app\casts\moneycast`, `App\Money`})
	d.set("file:///Invoice.php", []string{`App\Money`})
	if got := d.byModel["file:///Order.php"]; !slices.Equal(got, []string{`app\casts\moneycast`, `app\money`}) {
		t.Errorf("Order.php's classes = %q, want each once", got)
	}
	if got := d.of([]string{`APP\CASTS\MONEYCAST`}, ""); !slices.Equal(got, []string{"file:///Order.php"}) {
		t.Errorf("dependents of MoneyCast = %q, want Order.php", got)
	}
	if got := d.of([]string{`App\Money`}, "file:///Order.php"); !slices.Equal(got, []string{"file:///Invoice.php"}) {
		t.Errorf("dependents of Money other than Order.php = %q, want Invoice.php", got)
	}
}

// BenchmarkRefreshCastDependents measures the background work an edit to a
// cast class starts: re-analysing the one model whose casts use it.
func BenchmarkRefreshCastDependents(b *testing.B) {
	cw := newCastWorkspace(b)
	versions := [2]string{moneyCast(`\App\Price`), moneyCast(`\App\Money`)}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cw.ws.RefreshModels(cw.ws.IndexFile(cw.castURI, versions[i%2]))
	}
}
