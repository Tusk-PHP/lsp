package magento

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func magentoIndex() *symbols.Index {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Product.php", `<?php
namespace Magento\Catalog\Model;
class Product { public function getSku() {} }
`)
	idx.IndexFile("file:///ProductInterface.php", `<?php
namespace Magento\Catalog\Api\Data;
interface ProductInterface { public function getSku(); }
`)
	idx.SetClassGenerator(GeneratedClasses{})
	return idx
}

func TestGeneratedClasses(t *testing.T) {
	idx := magentoIndex()
	for _, tc := range []struct {
		fqn, kind, extends string
		implements         []string
	}{
		{`Magento\Catalog\Model\ProductFactory`, "class", "", nil},
		{`Magento\Catalog\Api\Data\ProductInterfaceFactory`, "class", "", nil},
		{`Magento\Catalog\Model\Product\Proxy`, "class", `Magento\Catalog\Model\Product`, []string{`Magento\Framework\ObjectManager\NoninterceptableInterface`}},
		{`Magento\Catalog\Api\Data\ProductInterface\Proxy`, "class", "", []string{`Magento\Catalog\Api\Data\ProductInterface`, `Magento\Framework\ObjectManager\NoninterceptableInterface`}},
		{`Magento\Catalog\Model\Product\Interceptor`, "class", `Magento\Catalog\Model\Product`, []string{`Magento\Framework\Interception\InterceptorInterface`}},
		{`Magento\Catalog\Model\ProductSearchResults`, "class", `Magento\Framework\Api\SearchResults`, nil},
		{`Magento\Catalog\Model\ProductSearchResultsFactory`, "class", "", nil},
		{`Magento\Catalog\Api\Data\ProductExtensionInterface`, "interface", "", []string{`Magento\Framework\Api\ExtensionAttributesInterface`}},
		{`Magento\Catalog\Api\Data\ProductExtension`, "class", `Magento\Framework\Api\AbstractSimpleObject`, []string{`Magento\Catalog\Api\Data\ProductExtensionInterface`}},
	} {
		sym := idx.Lookup(tc.fqn)
		if sym == nil {
			t.Errorf("%s not generated", tc.fqn)
			continue
		}
		wantKind := symbols.KindClass
		if tc.kind == "interface" {
			wantKind = symbols.KindInterface
		}
		if sym.Kind != wantKind || sym.Extends != tc.extends || len(sym.Implements) != len(tc.implements) {
			t.Errorf("%s = kind %v extends %q implements %q", tc.fqn, sym.Kind, sym.Extends, sym.Implements)
			continue
		}
		for i := range tc.implements {
			if sym.Implements[i] != tc.implements[i] {
				t.Errorf("%s implements %q, want %q", tc.fqn, sym.Implements, tc.implements)
			}
		}
	}

	create := idx.Lookup(`Magento\Catalog\Model\ProductFactory::create`)
	if create == nil || len(create.Params) != 1 || !strings.Contains(create.DocComment, `@return \Magento\Catalog\Model\Product`) {
		t.Fatalf("ProductFactory::create = %+v", create)
	}
	items := idx.Lookup(`Magento\Catalog\Model\ProductSearchResults::getItems`)
	if items == nil || !strings.Contains(items.DocComment, `@return \Magento\Catalog\Model\Product[]`) {
		t.Errorf("ProductSearchResults::getItems = %+v", items)
	}
}

func TestGeneratedClassesNeedTheirSource(t *testing.T) {
	idx := magentoIndex()
	for _, fqn := range []string{
		`Magento\Catalog\Model\MissingFactory`,
		`Magento\Catalog\Model\Missing\Proxy`,
		`Magento\Catalog\Api\Data\ProductInterface\Interceptor`, // interfaces aren't intercepted
		`Magento\Catalog\Model\ProductExtensionInterface`,       // no ProductInterface in Model
		`Magento\Catalog\Model\ProductExtension`,
		`Factory`,
		`\Proxy`,
	} {
		if sym := idx.Lookup(fqn); sym != nil {
			t.Errorf("%s generated, want unknown", fqn)
		}
	}
}
