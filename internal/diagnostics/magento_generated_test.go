package diagnostics

import (
	"sort"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/config"
	frameworkmagento "github.com/Tusk-PHP/lsp/internal/framework/magento"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Magento code uses factories, proxies and extension interfaces that only
// exist once `bin/magento setup:di:compile` has run.
func TestMagentoGeneratedClassesAreKnown(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Product.php", `<?php
namespace Magento\Catalog\Model;
class Product { public function getSku() {} }
`)
	idx.IndexFile("file:///ProductInterface.php", `<?php
namespace Magento\Catalog\Api\Data;
interface ProductInterface {
    /** @return \Magento\Catalog\Api\Data\ProductExtensionInterface|null */
    public function getExtensionAttributes();
}
`)
	idx.IndexFile("file:///AbstractSimpleObject.php", `<?php
namespace Magento\Framework\Api;
abstract class AbstractSimpleObject { public function __toArray() {} }
interface ExtensionAttributesInterface {}
`)
	idx.SetClassGenerator(frameworkmagento.GeneratedClasses{})

	src := `<?php
namespace Magento\Catalog\Controller;

use Magento\Catalog\Model\ProductFactory;
use Magento\Catalog\Api\Data\ProductInterface;
use Magento\Catalog\Api\Data\ProductExtensionInterface;

class Save {
    private ProductFactory $productFactory;

    public function __construct(ProductFactory $productFactory, \Magento\Catalog\Model\Product\Proxy $proxy) {
        $this->productFactory = $productFactory;
    }

    public function execute(ProductInterface $item, \Magento\Catalog\Api\Data\ProductExtension $ext): void {
        $product = $this->productFactory->create();
        $product->getSku();
        $product->missingMethod();
        $item->getExtensionAttributes()->getStockItem();
        $ext->getStockItem();
        $x = ProductExtensionInterface::class;
        $y = ProductFactory::class;
        $this->productFactory->build();
    }
}
`
	idx.IndexFile("file:///Save.php", src)
	idx.MarkReady()
	p := NewProvider(idx, "magento", "/tmp", nil, config.DefaultConfig())
	comp := completion.NewProvider(idx, nil, "magento")
	p.TypeResolver = func(expr, source string, line, col int, file *parser.FileNode) string {
		return comp.ResolveExpressionType(expr, source, protocol.Position{Line: line}, file)
	}
	var got []string
	for _, d := range p.Analyze("file:///Save.php", src) {
		if strings.HasPrefix(d.Message, "Unknown") {
			got = append(got, d.Message)
		}
	}
	sort.Strings(got)
	// The extension attribute getStockItem comes from extension_attributes.xml,
	// which isn't read: it stays quiet. Real mistakes are still reported.
	want := []string{
		"Unknown member 'build' on 'ProductFactory'",
		"Unknown member 'missingMethod' on 'Product'",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
