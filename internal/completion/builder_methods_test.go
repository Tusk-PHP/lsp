package completion

import (
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/models"
)

func TestBuilderColumnsLeaveOutAccessors(t *testing.T) {
	p, root := setupEloquentCompletionTest(t)
	src := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;

class Author extends Model {
    public string $name;

    public function getDisplayNameAttribute(): string
    {
        return $this->name;
    }
}
`
	writeProjectFile(t, root, "app/Models/Author.php", src)
	p.index.IndexFile("file://"+filepath.Join(root, "app/Models/Author.php"), src)
	models.AnalyzeEloquentModels(p.index, root)

	if labels := completeMarked(p, "<?php\nuse App\\Models\\Author;\n$a = new Author();\n$a->‸"); !labels["display_name"] {
		t.Fatalf("the accessor is a property of the model: got %v", mapKeys(labels, 20))
	}
	labels := completeMarked(p, "<?php\nuse App\\Models\\Author;\nAuthor::where('‸')")
	if !labels["name"] || labels["display_name"] {
		t.Errorf("where() takes columns, not accessors: got %v", mapKeys(labels, 20))
	}
}

func TestBuilderCompletesTheRelatedModelsColumnOfAnAggregate(t *testing.T) {
	p, _ := setupEloquentCompletionTest(t)
	for _, code := range []string{
		"<?php\nuse App\\Models\\Product;\nProduct::withSum('category', '‸')",
		"<?php\nuse App\\Models\\Product;\nProduct::withMax(\n    'category as top',\n    'sort_‸'\n)",
	} {
		labels := completeMarked(p, code)
		if !labels["sort_order"] || labels["price"] {
			t.Errorf("%q: want the columns of Category, got %v", code, mapKeys(labels, 20))
		}
	}
	for _, code := range []string{
		"<?php\nuse App\\Models\\Category;\nCategory::withSum('products', '‸')",
		"<?php\nuse App\\Models\\Product;\nProduct::withSum($relation, '‸')",
		"<?php\nuse App\\Models\\Product;\nProduct::withCount('category', '‸')",
		"<?php\nuse App\\Models\\Product;\nProduct::withSum('category', ['‸'])",
	} {
		if labels := completeMarked(p, code); labels["sort_order"] || labels["price"] {
			t.Errorf("%q: got %v", code, mapKeys(labels, 20))
		}
	}
}

func TestBuilderCompletesTheSecondColumnOfWhereColumnAndPluck(t *testing.T) {
	p, _ := setupEloquentCompletionTest(t)
	for _, code := range []string{
		"<?php\nuse App\\Models\\Product;\nProduct::whereColumn('price', '‸')",
		"<?php\nuse App\\Models\\Product;\nProduct::orWhereColumn('price', '>', '‸')",
		"<?php\nuse App\\Models\\Product;\nProduct::pluck('name', '‸')",
	} {
		if labels := completeMarked(p, code); !labels["price"] || labels["sort_order"] {
			t.Errorf("%q: want the columns of Product, got %v", code, mapKeys(labels, 20))
		}
	}
	for _, code := range []string{
		"<?php\nuse App\\Models\\Product;\nProduct::whereColumn('price', '‸', 'cost')",
		"<?php\nuse App\\Models\\Product;\nProduct::pluck('name', 'id', '‸')",
		"<?php\nuse App\\Models\\Product;\nProduct::pluck('name', ['‸'])",
		"<?php\nuse App\\Models\\Product;\nProduct::value('name', '‸')",
	} {
		if labels := completeMarked(p, code); labels["price"] {
			t.Errorf("%q: got %v", code, mapKeys(labels, 20))
		}
	}
}
