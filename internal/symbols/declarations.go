package symbols

import "github.com/Tusk-PHP/lsp/internal/parser"

func fileDeclarations(uri string, file *parser.FileNode) []*Symbol {
	decls := make([]*Symbol, 0, len(file.Classes)+len(file.AnonymousClasses)+len(file.Interfaces)+
		len(file.Traits)+len(file.Enums)+len(file.Constants)+len(file.Functions))
	for i := range file.Classes {
		decls = append(decls, classSymbol(uri, &file.Classes[i]))
	}
	for i := range file.AnonymousClasses {
		decls = append(decls, classSymbol(uri, &file.AnonymousClasses[i]))
	}
	for i := range file.Interfaces {
		decls = append(decls, interfaceSymbol(uri, &file.Interfaces[i]))
	}
	for i := range file.Traits {
		decls = append(decls, traitSymbol(uri, &file.Traits[i]))
	}
	for i := range file.Enums {
		decls = append(decls, enumSymbol(uri, &file.Enums[i]))
	}
	decls = appendGlobalConstants(decls, uri, file.Constants)
	for i := range file.Functions {
		decls = append(decls, FunctionSymbol(uri, &file.Functions[i]))
	}
	return decls
}

func classSymbol(uri string, c *parser.ClassNode) *Symbol {
	fqn := c.FullName
	resolve, uses := namespaceResolver(c.Namespace), c.Namespace.Uses
	sym := &Symbol{Name: c.Name, FQN: fqn, Kind: KindClass, URI: uri, DocComment: qualifyDocTypes(c.DocComment, uses),
		IsAbstract: c.IsAbstract, IsFinal: c.IsFinal, IsReadonly: c.IsReadonly, IsConditional: c.Conditional,
		Implements: resolveNames(c.Implements, resolve), Traits: resolveNames(c.Traits, resolve), traitRules: newTraitRules(fqn, c.TraitRules, resolve),
		Attributes: indexAttributes(c.Attributes, resolve),
		Range:      symRange(c.StartLine, c.StartCol, len(c.Name))}
	if c.Extends != "" {
		sym.Extends = resolve(c.Extends)
	}
	addProperties(uri, sym, c.Properties, resolve, uses)
	addMethods(uri, sym, c.Methods, resolve, uses)
	addClassConstants(uri, sym, c.Constants, resolve, uses)
	addVirtualMembers(uri, sym, resolve)
	sym.Supertypes = append(sym.Supertypes, traitUseSupertypes(c.TraitUseDoc, uses, resolve)...)
	return sym
}

func interfaceSymbol(uri string, iface *parser.InterfaceNode) *Symbol {
	resolve, uses := namespaceResolver(iface.Namespace), iface.Namespace.Uses
	sym := &Symbol{Name: iface.Name, FQN: iface.FullName, Kind: KindInterface, URI: uri, DocComment: qualifyDocTypes(iface.DocComment, uses),
		Implements: resolveNames(iface.Extends, resolve), Attributes: indexAttributes(iface.Attributes, resolve),
		Range: symRange(iface.StartLine, iface.StartCol, len(iface.Name))}
	addProperties(uri, sym, iface.Properties, resolve, uses)
	addMethods(uri, sym, iface.Methods, resolve, uses)
	addClassConstants(uri, sym, iface.Constants, resolve, uses)
	addVirtualMembers(uri, sym, resolve)
	return sym
}

func traitSymbol(uri string, tr *parser.TraitNode) *Symbol {
	fqn := tr.FullName
	resolve, uses := namespaceResolver(tr.Namespace), tr.Namespace.Uses
	sym := &Symbol{Name: tr.Name, FQN: fqn, Kind: KindTrait, URI: uri, DocComment: qualifyDocTypes(tr.DocComment, uses),
		Traits: resolveNames(tr.Traits, resolve), traitRules: newTraitRules(fqn, tr.TraitRules, resolve),
		Range: symRange(tr.StartLine, tr.StartCol, len(tr.Name))}
	addProperties(uri, sym, tr.Properties, resolve, uses)
	addMethods(uri, sym, tr.Methods, resolve, uses)
	addClassConstants(uri, sym, tr.Constants, resolve, uses)
	addVirtualMembers(uri, sym, resolve)
	sym.Supertypes = append(sym.Supertypes, traitUseSupertypes(tr.TraitUseDoc, uses, resolve)...)
	return sym
}

func enumSymbol(uri string, en *parser.EnumNode) *Symbol {
	fqn := en.FullName
	resolve, uses := namespaceResolver(en.Namespace), en.Namespace.Uses
	sym := &Symbol{Name: en.Name, FQN: fqn, Kind: KindEnum, URI: uri, DocComment: qualifyDocTypes(en.DocComment, uses),
		BackedType: en.BackedType, Implements: resolveNames(en.Implements, resolve), Traits: resolveNames(en.Traits, resolve),
		traitRules: newTraitRules(fqn, en.TraitRules, resolve),
		Attributes: indexAttributes(en.Attributes, resolve),
		Range:      symRange(en.StartLine, en.StartCol, len(en.Name))}
	for _, ec := range en.Cases {
		sym.Children = append(sym.Children, &Symbol{Name: ec.Name, FQN: fqn + "::" + ec.Name, Kind: KindEnumCase, URI: uri, ParentFQN: fqn, Value: ec.Value,
			DocComment: ec.DocComment, Attributes: indexAttributes(ec.Attributes, resolve),
			Range: symRange(ec.StartLine, ec.StartCol, len(ec.Name))})
	}
	addProperties(uri, sym, en.Properties, resolve, uses)
	addMethods(uri, sym, en.Methods, resolve, uses)
	addClassConstants(uri, sym, en.Constants, resolve, uses)
	addEnumMembers(uri, sym)
	return sym
}

// FunctionSymbol is the symbol the index keeps for a function declared in
// file, with its names resolved against that file's namespace and imports.
func FunctionSymbol(uri string, fn *parser.FunctionNode) *Symbol {
	resolve := namespaceResolver(fn.Namespace)
	sym := &Symbol{Name: fn.Name, FQN: fn.FullName, Kind: KindFunction, URI: uri, ReturnType: resolve(fn.ReturnType.Name),
		Attributes: indexAttributes(fn.Attributes, resolve),
		DocComment: qualifyDocTypes(fn.DocComment, fn.Namespace.Uses), Range: symRange(fn.StartLine, fn.StartCol, len(fn.Name))}
	sym.Templates = docTemplates(sym.DocComment, resolve)
	if len(fn.Params) > 0 {
		sym.Params = make([]ParamInfo, 0, len(fn.Params))
	}
	for _, p := range fn.Params {
		sym.Params = append(sym.Params, ParamInfo{Name: p.Name, Type: resolve(p.Type.Name), DefaultValue: p.DefaultValue, IsVariadic: p.IsVariadic, IsReference: p.IsReference})
	}
	return sym
}

func addMethods(uri string, parent *Symbol, methods []parser.MethodNode, resolve func(string) string, uses []parser.UseNode) {
	for _, m := range methods {
		ms := &Symbol{
			Name:       m.Name,
			FQN:        parent.FQN + "::" + m.Name,
			Kind:       KindMethod,
			Attributes: indexAttributes(m.Attributes, resolve),
			URI:        uri,
			Visibility: m.Visibility,
			IsStatic:   m.IsStatic,
			IsAbstract: m.IsAbstract,
			IsFinal:    m.IsFinal,
			ReturnType: resolve(m.ReturnType.Name),
			DocComment: qualifyDocTypes(m.DocComment, uses),
			ParentFQN:  parent.FQN,
			Range:      symRange(m.StartLine, m.StartCol, len(m.Name)),
		}
		ms.Templates = docTemplates(ms.DocComment, resolve)
		for _, p := range m.Params {
			ms.Params = append(ms.Params, ParamInfo{
				Name:         p.Name,
				Type:         resolve(p.Type.Name),
				DefaultValue: p.DefaultValue,
				IsVariadic:   p.IsVariadic,
				IsReference:  p.IsReference,
			})
		}
		parent.Children = append(parent.Children, ms)
	}
}

func addProperties(uri string, parent *Symbol, props []parser.PropertyNode, resolve func(string) string, uses []parser.UseNode) {
	for _, prop := range props {
		parent.Children = append(parent.Children, &Symbol{Name: prop.Name, FQN: parent.FQN + "::" + prop.Name, Kind: KindProperty, URI: uri,
			Visibility: prop.Visibility, SetVisibility: prop.SetVisibility,
			IsStatic: prop.IsStatic, IsReadonly: prop.IsReadonly, Type: resolve(prop.Type.Name),
			DocComment: qualifyDocTypes(prop.DocComment, uses), ParentFQN: parent.FQN, Attributes: indexAttributes(prop.Attributes, resolve),
			Hooks: append([]parser.PropertyHook(nil), prop.Hooks...),
			Range: symRange(prop.StartLine, prop.StartCol, len(prop.Name))})
	}
}

// addEnumMembers adds what PHP gives every enum: `name` and `cases()`, and for
// backed enums `value`, `from()` and `tryFrom()`. They point at the enum itself.
func addEnumMembers(uri string, enum *Symbol) {
	add := func(member *Symbol) {
		member.FQN = enum.FQN + "::" + member.Name
		member.URI = uri
		member.ParentFQN = enum.FQN
		member.Visibility = "public"
		member.Range = enum.Range
		enum.Children = append(enum.Children, member)
	}
	add(&Symbol{Name: "$name", Kind: KindProperty, Type: "string", IsReadonly: true})
	add(&Symbol{Name: "cases", Kind: KindMethod, IsStatic: true, ReturnType: "array",
		DocComment: "/** @return " + enum.FQN + "[] */"})
	if enum.BackedType == "" {
		return
	}
	value := []ParamInfo{{Name: "$value", Type: "int|string"}}
	add(&Symbol{Name: "$value", Kind: KindProperty, Type: enum.BackedType, IsReadonly: true})
	add(&Symbol{Name: "from", Kind: KindMethod, IsStatic: true, ReturnType: enum.FQN, Params: value})
	add(&Symbol{Name: "tryFrom", Kind: KindMethod, IsStatic: true, ReturnType: "?" + enum.FQN, Params: value})
}

// addVirtualMembers parses @property/@method docblock tags on a class-level
// symbol and injects them as virtual children so they appear in completion/hover.
func addVirtualMembers(uri string, parent *Symbol, resolve func(string) string) {
	if parent.DocComment == "" {
		return
	}
	doc := parser.ParseDocBlock(parent.DocComment)
	if doc == nil {
		return
	}
	fqn := parent.FQN

	indexTypeAliases(parent, doc, resolve)
	parent.Templates = templateParams(doc, resolve)
	parent.Supertypes = supertypes(doc, resolve)

	for _, prop := range doc.Properties {
		if prop.Name == "" {
			continue
		}
		propName := "$" + prop.Name
		parent.Children = append(parent.Children, &Symbol{
			Name:       propName,
			FQN:        fqn + "::" + propName,
			Kind:       KindProperty,
			URI:        uri,
			Visibility: "public",
			Type:       resolve(prop.Type),
			ParentFQN:  fqn,
			IsVirtual:  true,
			DocComment: prop.Description,
		})
	}

	for _, method := range doc.Methods {
		if method.Name == "" {
			continue
		}
		ms := &Symbol{
			Name:       method.Name,
			FQN:        fqn + "::" + method.Name,
			Kind:       KindMethod,
			URI:        uri,
			Visibility: "public",
			ReturnType: resolve(method.ReturnType),
			ParentFQN:  fqn,
			IsVirtual:  true,
			IsStatic:   method.IsStatic,
			DocComment: method.Description,
		}
		if method.Params != "" {
			ms.Params = parseDocMethodParams(method.Params, resolve)
		}
		parent.Children = append(parent.Children, ms)
	}
}
