package diagnostics

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestCastAttributesStayColumns(t *testing.T) {
	const uri = "file:///app/Models/Order.php"
	const source = `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Order extends Model {
    protected $casts = ['paid_at' => 'datetime', 'options' => 'array'];
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", "<?php\nnamespace Illuminate\\Database\\Eloquent;\nabstract class Model {}\n")
	idx.IndexFile(uri, source)
	schema := &models.Schema{Source: models.SchemaSourceLive, Tables: []models.SchemaTable{{
		Name: "orders",
		Columns: []models.SchemaColumn{
			{Name: "id", DataType: "bigint", ColumnType: "bigint(20)"},
			{Name: "paid_at", DataType: "timestamp", ColumnType: "timestamp", IsNullable: true},
		},
	}}}
	models.AnalyzeModelFile(idx, "/", "laravel", uri, source, schema, nil)
	checker := NewIndexMemberChecker(idx, func() bool { return true })
	const order = `App\Models\Order`
	if !checker.KnowsColumns(order) {
		t.Fatal("the live schema's columns should still be known")
	}
	for _, name := range []string{"paid_at", "options"} {
		if !checker.IsColumn(order, name) || !checker.IsDBColumn(order, name) {
			t.Errorf("cast attribute %s should count as a column", name)
		}
	}
}
