// Package magento holds what Tusk PHP knows about Magento 2.
package magento

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// GeneratedClasses supplies the classes Magento writes to generated/code when
// it compiles dependency injection (`bin/magento setup:di:compile`), or on
// first use in developer mode. Code refers to them long before they exist:
// `ProductFactory`, `Product\Proxy`, `Product\Interceptor`,
// `ProductSearchResults`, `ProductExtensionInterface`, `ProductExtension`.
//
// Each one is built from the class it's generated for, with the members
// Magento gives it. The getters and setters of extension attributes come from
// extension_attributes.xml, which isn't read, so their generated classes list
// none of them.
type GeneratedClasses struct{}

const (
	suffixFactory            = "Factory"
	suffixProxy              = `\Proxy`
	suffixInterceptor        = `\Interceptor`
	suffixSearchResults      = "SearchResults"
	suffixExtension          = "Extension"
	suffixExtensionInterface = "ExtensionInterface"
)

// Candidate implements symbols.ClassGenerator.
func (GeneratedClasses) Candidate(fqn string) bool {
	return strings.HasSuffix(fqn, suffixFactory) ||
		strings.HasSuffix(fqn, suffixProxy) ||
		strings.HasSuffix(fqn, suffixInterceptor) ||
		strings.HasSuffix(fqn, suffixSearchResults) ||
		strings.HasSuffix(fqn, suffixExtension) ||
		strings.HasSuffix(fqn, suffixExtensionInterface)
}

// Generate implements symbols.ClassGenerator.
func (GeneratedClasses) Generate(fqn string, idx *symbols.Index) (string, string, bool) {
	switch {
	case strings.HasSuffix(fqn, suffixProxy):
		source := sourceClass(idx, strings.TrimSuffix(fqn, suffixProxy), classOrInterface)
		if source == nil {
			return "", "", false
		}
		// A proxy of an interface implements it; of a class, extends it.
		decl := "class Proxy extends \\" + source.FQN + " implements \\Magento\\Framework\\ObjectManager\\NoninterceptableInterface"
		if source.Kind == symbols.KindInterface {
			decl = "class Proxy implements \\" + source.FQN + ", \\Magento\\Framework\\ObjectManager\\NoninterceptableInterface"
		}
		return phpClass(fqn, "Proxy class for @see \\"+source.FQN, decl, ""), source.FQN, true

	case strings.HasSuffix(fqn, suffixInterceptor):
		source := sourceClass(idx, strings.TrimSuffix(fqn, suffixInterceptor), classOnly)
		if source == nil {
			return "", "", false
		}
		decl := "class Interceptor extends \\" + source.FQN + " implements \\Magento\\Framework\\Interception\\InterceptorInterface"
		return phpClass(fqn, "Interceptor class for @see \\"+source.FQN, decl, ""), source.FQN, true

	case strings.HasSuffix(fqn, suffixSearchResults):
		source := sourceClass(idx, strings.TrimSuffix(fqn, suffixSearchResults), classOrInterface)
		if source == nil {
			return "", "", false
		}
		decl := "class " + shortName(fqn) + " extends \\Magento\\Framework\\Api\\SearchResults"
		body := "    /**\n     * Returns array of items\n     *\n     * @return \\" + source.FQN + "[]\n     */\n    public function getItems() {}\n"
		return phpClass(fqn, "SearchResults class for @see \\"+source.FQN, decl, body), source.FQN, true

	case strings.HasSuffix(fqn, suffixExtensionInterface):
		// ProductInterface → ProductExtensionInterface
		source := sourceClass(idx, strings.TrimSuffix(fqn, suffixExtensionInterface)+"Interface", interfaceOnly)
		if source == nil {
			return "", "", false
		}
		decl := "interface " + shortName(fqn) + " extends \\Magento\\Framework\\Api\\ExtensionAttributesInterface"
		return phpClass(fqn, "ExtensionInterface class for @see \\"+source.FQN, decl, ""), source.FQN, true

	case strings.HasSuffix(fqn, suffixExtension):
		// ProductInterface → ProductExtension, which implements ProductExtensionInterface
		base := strings.TrimSuffix(fqn, suffixExtension)
		source := sourceClass(idx, base+"Interface", interfaceOnly)
		if source == nil {
			return "", "", false
		}
		decl := "class " + shortName(fqn) + " extends \\Magento\\Framework\\Api\\AbstractSimpleObject implements \\" + fqn + "Interface"
		return phpClass(fqn, "Extension class for @see \\"+source.FQN, decl, ""), source.FQN, true

	case strings.HasSuffix(fqn, suffixFactory):
		source := sourceClass(idx, strings.TrimSuffix(fqn, suffixFactory), classOrInterface)
		if source == nil {
			return "", "", false
		}
		target := "\\" + source.FQN
		var body strings.Builder
		body.WriteString("    /**\n     * Factory constructor\n     *\n")
		body.WriteString("     * @param \\Magento\\Framework\\ObjectManagerInterface $objectManager\n")
		body.WriteString("     * @param string $instanceName\n     */\n")
		body.WriteString("    public function __construct(\\Magento\\Framework\\ObjectManagerInterface $objectManager, $instanceName = '")
		body.WriteString(strings.ReplaceAll(target, `\`, `\\`))
		body.WriteString("') {}\n\n")
		body.WriteString("    /**\n     * Create class instance with specified parameters\n     *\n")
		body.WriteString("     * @param array $data\n     * @return ")
		body.WriteString(target)
		body.WriteString("\n     */\n    public function create(array $data = []) {}\n")
		decl := "class " + shortName(fqn)
		return phpClass(fqn, "Factory class for @see "+target, decl, body.String()), source.FQN, true
	}
	return "", "", false
}

type sourceKind int

const (
	classOrInterface sourceKind = iota
	classOnly
	interfaceOnly
)

// sourceClass finds the class a generated class is built from.
func sourceClass(idx *symbols.Index, fqn string, want sourceKind) *symbols.Symbol {
	if fqn == "" || strings.HasSuffix(fqn, `\`) {
		return nil
	}
	sym := idx.LookupFold(fqn, func(s *symbols.Symbol) bool {
		switch s.Kind {
		case symbols.KindClass:
			return want != interfaceOnly
		case symbols.KindInterface:
			return want != classOnly
		}
		return false
	})
	return sym
}

// phpClass writes the source of a generated class: decl is its declaration line
// and body what goes between its braces.
func phpClass(fqn, doc, decl, body string) string {
	var b strings.Builder
	b.Grow(len(fqn) + len(doc) + len(decl) + len(body) + 64)
	b.WriteString("<?php\n")
	if i := strings.LastIndex(fqn, `\`); i > 0 {
		b.WriteString("namespace ")
		b.WriteString(fqn[:i])
		b.WriteString(";\n\n")
	}
	b.WriteString("/**\n * ")
	b.WriteString(doc)
	b.WriteString("\n */\n")
	b.WriteString(decl)
	b.WriteString("\n{\n")
	b.WriteString(body)
	b.WriteString("}\n")
	return b.String()
}

func shortName(fqn string) string {
	if i := strings.LastIndex(fqn, `\`); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}
