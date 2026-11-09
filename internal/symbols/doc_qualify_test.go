package symbols

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func TestQualifyDocTypes(t *testing.T) {
	uses := []parser.UseNode{
		{FullName: "Magento\\Catalog\\Model\\Product", Alias: "Product", Kind: "class"},
		{FullName: "Illuminate\\Support\\Collection", Alias: "Collection"},
		{FullName: "App\\Models", Alias: "Models"},
		{FullName: "App\\helper", Alias: "helper", Kind: "function"},
	}
	tests := []struct{ name, in, want string }{
		{"return", "/** @return Product */", "/** @return \\Magento\\Catalog\\Model\\Product */"},
		{"nullable union", "/** @return ?Product|null */", "/** @return ?\\Magento\\Catalog\\Model\\Product|null */"},
		{"generic and template", "/** @return Collection<int, TValue> */", "/** @return \\Illuminate\\Support\\Collection<int, TValue> */"},
		{"param keeps variable and description", "/**\n * @param Product $product The Product to save\n */",
			"/**\n * @param \\Magento\\Catalog\\Model\\Product $product The Product to save\n */"},
		{"var", "/** @var Product[] */", "/** @var \\Magento\\Catalog\\Model\\Product[] */"},
		{"phpstan prefix", "/** @phpstan-return list<Product> */", "/** @phpstan-return list<\\Magento\\Catalog\\Model\\Product> */"},
		{"namespace import", "/** @return Models\\User */", "/** @return \\App\\Models\\User */"},
		{"shape keys stay", "/** @return array{Product: Product, Collection?: int} */",
			"/** @return array{Product: \\Magento\\Catalog\\Model\\Product, Collection?: int} */"},
		{"class constant", "/** @param Product::TYPE_* $t */", "/** @param \\Magento\\Catalog\\Model\\Product::TYPE_* $t */"},
		{"quoted literal", "/** @param 'Product'|Product $p */", "/** @param 'Product'|\\Magento\\Catalog\\Model\\Product $p */"},
		{"fully qualified stays", "/** @return \\Other\\Product */", "/** @return \\Other\\Product */"},
		{"not imported stays", "/** @return Order|TModel */", "/** @return Order|TModel */"},
		{"function import ignored", "/** @return helper */", "/** @return helper */"},
		{"other tags stay", "/** @see Product\n * Product description */", "/** @see Product\n * Product description */"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qualifyDocTypes(tt.in, uses); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestQualifyDocTypesNoImportsDoesNotAllocate(t *testing.T) {
	uses := []parser.UseNode{{FullName: "A\\B", Alias: "B"}}
	doc := "/**\n * @param string $x\n * @return int\n */"
	if n := testing.AllocsPerRun(100, func() { qualifyDocTypes(doc, uses) }); n != 0 {
		t.Errorf("allocations = %v, want 0", n)
	}
}

func BenchmarkQualifyDocTypes(b *testing.B) {
	uses := []parser.UseNode{
		{FullName: "Magento\\Catalog\\Model\\Product", Alias: "Product"},
		{FullName: "Magento\\Framework\\Exception\\LocalizedException", Alias: "LocalizedException"},
		{FullName: "Psr\\Log\\LoggerInterface", Alias: "LoggerInterface"},
	}
	imported := "/**\n * Initialize the product.\n *\n * @param int $id\n * @param Product|null $product\n * @return Product\n * @throws LocalizedException\n */"
	plain := "/**\n * Initialize the product.\n *\n * @param int $id\n * @param array<string, mixed> $data\n * @return bool\n */"
	b.Run("imported", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			qualifyDocTypes(imported, uses)
		}
	})
	b.Run("plain", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			qualifyDocTypes(plain, uses)
		}
	})
}
