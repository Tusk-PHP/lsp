package parser

import (
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func ParseFile(source string, version token.Version) *FileNode {
	return ParseFileIn(source, phpparser.OnlyGrammar(version))
}

func ParseFileIn(source string, grammars phpparser.GrammarRange) *FileNode {
	return FileNodeOf(source, ParseTreeIn(source, grammars))
}

func ParseTree(source string, version token.Version) *ast.File {
	return ParseTreeIn(source, phpparser.OnlyGrammar(version))
}

func ParseTreeIn(source string, grammars phpparser.GrammarRange) *ast.File {
	if !HasOpenTag(source) {
		return grammars.ParseCode(source)
	}
	return grammars.Parse(source)
}

type declScope int

const (
	scopeFile declScope = iota
	scopeBlock
	scopeFunction
	scopeClass
)

func (s declScope) declaresClasses() bool {
	return s == scopeFile || s == scopeBlock
}

func (s declScope) declaresFunctions() bool {
	return s != scopeClass
}

type fileNodeBuilder struct {
	src      string
	lines    ast.Lines
	file     *FileNode
	fileHash string
}

func FileNodeOf(src string, tree *ast.File) *FileNode {
	b := &fileNodeBuilder{src: src, lines: ast.NewLines(src), file: &FileNode{Source: src, Tree: tree, Grammar: tree.Version}}
	for _, stmt := range tree.Stmts {
		b.visit(stmt, scopeFile)
	}
	for _, e := range tree.Errors {
		line, col := b.position(e.Start)
		b.file.Errors = append(b.file.Errors, ParseError{Message: e.Message, Line: line, Column: col})
	}
	return b.file
}

func (b *fileNodeBuilder) position(offset uint32) (line, col int) {
	return b.lines.Position(offset)
}

func (b *fileNodeBuilder) line(offset uint32) int {
	line, _ := b.position(offset)
	return line
}

func (b *fileNodeBuilder) endLine(s ast.Span) int {
	return b.line(s.End - 1)
}

func (b *fileNodeBuilder) text(n ast.Node) string {
	return n.Bounds().Text(b.src)
}

func (b *fileNodeBuilder) doc(s ast.Span) string {
	return s.Text(b.src)
}

func (b *fileNodeBuilder) qualify(name string) string {
	if b.file.Namespace != "" {
		return b.file.Namespace + "\\" + name
	}
	return name
}

func (b *fileNodeBuilder) visit(n ast.Node, scope declScope) {
	switch n := n.(type) {
	case *ast.Namespace:
		b.file.Namespace = b.nameText(n.Name)
		for _, stmt := range n.Stmts {
			b.visit(stmt, scope)
		}
		return
	case *ast.Use:
		b.uses(n)
		return
	case *ast.Const:
		if scope.declaresClasses() {
			b.globalConstants(n)
		}
		return
	case *ast.FunctionDecl:
		if scope.declaresFunctions() {
			b.file.Functions = append(b.file.Functions, b.function(n))
			scope = scopeFunction
		}
	case *ast.ClassDecl:
		if scope.declaresClasses() {
			b.file.Classes = append(b.file.Classes, b.class(n, scope == scopeBlock))
		}
		scope = scopeClass
	case *ast.InterfaceDecl:
		if scope.declaresClasses() {
			b.file.Interfaces = append(b.file.Interfaces, b.iface(n))
		}
		scope = scopeClass
	case *ast.TraitDecl:
		if scope.declaresClasses() {
			b.file.Traits = append(b.file.Traits, b.trait(n))
		}
		scope = scopeClass
	case *ast.EnumDecl:
		if scope.declaresClasses() {
			b.file.Enums = append(b.file.Enums, b.enum(n))
		}
		scope = scopeClass
	case *ast.New:
		if n.Anonymous != nil {
			b.file.AnonymousClasses = append(b.file.AnonymousClasses, b.anonymousClass(n.Anonymous))
			args := n.Anonymous.Args
			ast.Children(n.Anonymous, func(c ast.Node) {
				if args != nil && c == ast.Node(args) {
					b.visit(c, scope)
					return
				}
				b.visit(c, scopeClass)
			})
			return
		}
	case *ast.Closure, *ast.ArrowFunction:
		if scope != scopeClass {
			scope = scopeFunction
		}
	}
	if scope == scopeFile {
		scope = scopeBlock
	}
	ast.Children(n, func(c ast.Node) { b.visit(c, scope) })
}

func (b *fileNodeBuilder) uses(u *ast.Use) {
	prefix := ""
	if u.Prefix != nil {
		prefix = b.text(u.Prefix) + "\\"
	}
	for _, item := range u.Items {
		if item.Name == nil {
			continue
		}
		fullName := strings.TrimPrefix(prefix+b.text(item.Name), "\\")
		alias := fullName[strings.LastIndexByte(fullName, '\\')+1:]
		if item.Alias != nil {
			alias = b.text(item.Alias)
		}
		b.file.Uses = append(b.file.Uses, UseNode{FullName: fullName, Alias: alias, Kind: useKind(item.Kind), StartLine: b.line(item.Name.Start)})
	}
}

func useKind(k token.Kind) string {
	switch k {
	case token.Function:
		return "function"
	case token.Const:
		return "const"
	}
	return "class"
}

func (b *fileNodeBuilder) globalConstants(c *ast.Const) {
	for _, item := range c.Items {
		if item.Name == nil {
			continue
		}
		b.file.Constants = append(b.file.Constants, b.constant(item, c.Doc, nil, "", nil))
	}
}

func (b *fileNodeBuilder) constant(item *ast.ConstItem, doc ast.Span, typ ast.Type, visibility string, attrs []*ast.AttributeGroup) ConstantNode {
	line, col := b.position(item.Name.Start)
	value := b.compact(item.Value)
	return ConstantNode{
		Name:       b.text(item.Name),
		Value:      value,
		ValueType:  b.literalType(item.Value, value),
		Type:       TypeNode{Name: b.typeText(typ)},
		Visibility: visibility,
		Attributes: b.attributes(attrs),
		DocComment: b.doc(doc),
		StartLine:  line,
		StartCol:   col,
	}
}

func (b *fileNodeBuilder) attributes(groups []*ast.AttributeGroup) []AttributeDef {
	var attrs []AttributeDef
	for _, group := range groups {
		for _, attr := range group.Attrs {
			if attr.Name == nil {
				continue
			}
			def := AttributeDef{Name: b.text(attr.Name), Line: b.line(attr.Name.Start)}
			if attr.Args != nil {
				for _, arg := range attr.Args.Args {
					def.Args = append(def.Args, b.attributeArg(arg))
				}
			}
			attrs = append(attrs, def)
		}
	}
	return attrs
}

func (b *fileNodeBuilder) attributeArg(arg *ast.Arg) AttributeArg {
	value := b.compact(arg.Value)
	if arg.Unpack {
		value = "..." + value
	}
	if arg.Name == nil {
		return AttributeArg{Value: value}
	}
	return AttributeArg{Name: b.text(arg.Name), Value: value}
}

func (b *fileNodeBuilder) function(fn *ast.FunctionDecl) FunctionNode {
	node := FunctionNode{
		Attributes: b.attributes(fn.Attrs),
		EndLine:    b.endLine(fn.Span),
		ReturnType: TypeNode{Name: b.typeText(fn.ReturnType)},
		DocComment: b.doc(fn.Doc),
		Params:     b.params(fn.Params),
	}
	if fn.Name != nil {
		node.Name = b.text(fn.Name)
		node.FullName = b.qualify(node.Name)
		node.StartLine, node.StartCol = b.position(fn.Name.Start)
	}
	return node
}

func (b *fileNodeBuilder) params(params []*ast.Param) []ParamNode {
	var nodes []ParamNode
	for _, p := range params {
		if p.Var == nil {
			continue
		}
		def := b.compact(p.Default)
		nodes = append(nodes, ParamNode{
			Name:         b.text(p.Var),
			Attributes:   b.attributes(p.Attrs),
			Type:         TypeNode{Name: b.typeText(p.Type)},
			DefaultValue: def,
			DefaultType:  b.literalType(p.Default, def),
			IsVariadic:   p.Variadic,
			IsReference:  p.ByRef,
			DocComment:   b.doc(p.Doc),
		})
	}
	return nodes
}

type modifierSet struct {
	visibility    string
	setVisibility string
	static        bool
	abstract      bool
	final         bool
	readonly      bool
	promotes      bool
}

func modifiers(mods []*ast.Modifier) modifierSet {
	set := modifierSet{visibility: "public"}
	visible := false
	for _, m := range mods {
		set.promotes = true
		switch m.Kind {
		case token.Public, token.Protected, token.Private:
			if !visible {
				set.visibility = m.Kind.Text()
				visible = true
			}
		case token.PublicSet:
			set.setVisibility = token.Public.Text()
		case token.ProtectedSet:
			set.setVisibility = token.Protected.Text()
		case token.PrivateSet:
			set.setVisibility = token.Private.Text()
		case token.Static:
			set.static = true
		case token.Abstract:
			set.abstract = true
		case token.Final:
			set.final = true
		case token.Readonly:
			set.readonly = true
		}
	}
	return set
}

type classBody struct {
	methods     []MethodNode
	properties  []PropertyNode
	constants   []ConstantNode
	cases       []EnumCaseNode
	traits      []string
	traitUseDoc string
	traitRules  TraitAdaptations
}

func (b *fileNodeBuilder) body(members []ast.Member, readonly bool) classBody {
	var body classBody
	var traitDocs []string
	for _, member := range members {
		switch m := member.(type) {
		case *ast.Method:
			method := b.method(m)
			body.methods = append(body.methods, method)
			if strings.EqualFold(method.Name, "__construct") {
				body.properties = append(body.properties, b.promoted(m.Params, readonly)...)
			}
		case *ast.Property:
			body.properties = append(body.properties, b.properties(m, readonly)...)
		case *ast.ClassConst:
			mods := modifiers(m.Modifiers)
			for _, item := range m.Items {
				if item.Name != nil {
					body.constants = append(body.constants, b.constant(item, m.Doc, m.Type, mods.visibility, m.Attrs))
				}
			}
		case *ast.EnumCase:
			if m.Name == nil {
				continue
			}
			line, col := b.position(m.Name.Start)
			body.cases = append(body.cases, EnumCaseNode{
				Name:       b.text(m.Name),
				Value:      b.compact(m.Value),
				Attributes: b.attributes(m.Attrs),
				DocComment: b.doc(m.Doc),
				StartLine:  line,
				StartCol:   col,
			})
		case *ast.TraitUse:
			if doc := b.doc(m.Doc); doc != "" {
				traitDocs = append(traitDocs, doc)
			}
			for _, name := range m.Traits {
				if name != nil {
					body.traits = append(body.traits, b.text(name))
				}
			}
			for _, a := range m.Adaptations {
				b.traitAdaptation(a, &body.traitRules)
			}
		}
	}
	body.traitUseDoc = strings.Join(traitDocs, "\n")
	return body
}

func (b *fileNodeBuilder) traitAdaptation(a *ast.TraitAdaptation, rules *TraitAdaptations) {
	if a.Method == nil {
		return
	}
	trait, method := b.nameText(a.Trait), b.text(a.Method)
	if len(a.InsteadOf) > 0 {
		if trait != "" {
			rules.Precedences = append(rules.Precedences, TraitPrecedence{Trait: trait, Method: method, InsteadOf: b.names(a.InsteadOf)})
		}
		return
	}
	rule := TraitAlias{Trait: trait, Method: method}
	if a.Alias != nil {
		rule.Alias = b.text(a.Alias)
	}
	if a.Modifier != nil {
		switch a.Modifier.Kind {
		case token.Public, token.Protected, token.Private:
			rule.Visibility = a.Modifier.Kind.Text()
		case token.Final:
			rule.Final = true
		}
	}
	if rule.Alias != "" || rule.Visibility != "" || rule.Final {
		rules.Aliases = append(rules.Aliases, rule)
	}
}

func (b *fileNodeBuilder) method(m *ast.Method) MethodNode {
	mods := modifiers(m.Modifiers)
	node := MethodNode{
		Attributes: b.attributes(m.Attrs),
		Params:     b.params(m.Params),
		ReturnType: TypeNode{Name: b.typeText(m.ReturnType)},
		Visibility: mods.visibility,
		IsStatic:   mods.static,
		IsAbstract: mods.abstract,
		IsFinal:    mods.final,
		DocComment: b.doc(m.Doc),
		EndLine:    b.endLine(m.Span),
	}
	if m.Name != nil {
		node.Name = b.text(m.Name)
		node.StartLine, node.StartCol = b.position(m.Name.Start)
	}
	return node
}

func (b *fileNodeBuilder) properties(p *ast.Property, readonly bool) []PropertyNode {
	mods := modifiers(p.Modifiers)
	var nodes []PropertyNode
	for _, item := range p.Items {
		if item.Var == nil {
			continue
		}
		line, col := b.position(item.Var.Start)
		nodes = append(nodes, PropertyNode{
			Name:          b.text(item.Var),
			DefaultValue:  b.scalar(item.Default),
			Type:          TypeNode{Name: b.typeText(p.Type)},
			Visibility:    mods.visibility,
			SetVisibility: mods.setVisibility,
			IsStatic:      mods.static,
			IsReadonly:    mods.readonly || readonly,
			Attributes:    b.attributes(p.Attrs),
			DocComment:    b.doc(p.Doc),
			StartLine:     line,
			StartCol:      col,
			Hooks:         b.hooks(p.Hooks),
		})
	}
	return nodes
}

func (b *fileNodeBuilder) promoted(params []*ast.Param, readonly bool) []PropertyNode {
	var nodes []PropertyNode
	for _, p := range params {
		mods := modifiers(p.Modifiers)
		if !mods.promotes || p.Var == nil {
			continue
		}
		line, col := b.position(p.Var.Start)
		nodes = append(nodes, PropertyNode{
			Name:          b.text(p.Var),
			DefaultValue:  b.scalar(p.Default),
			Type:          TypeNode{Name: b.typeText(p.Type)},
			Visibility:    mods.visibility,
			SetVisibility: mods.setVisibility,
			IsReadonly:    mods.readonly || readonly,
			Attributes:    b.attributes(p.Attrs),
			DocComment:    b.doc(p.Doc),
			StartLine:     line,
			StartCol:      col,
			Hooks:         b.hooks(p.Hooks),
		})
	}
	return nodes
}

func (b *fileNodeBuilder) hooks(hooks []*ast.PropertyHook) []PropertyHook {
	var kinds []PropertyHook
	for _, h := range hooks {
		if h.Name == nil {
			continue
		}
		if kind := strings.ToLower(b.text(h.Name)); kind == "get" || kind == "set" {
			kinds = append(kinds, PropertyHook{Kind: kind})
		}
	}
	return kinds
}

func (b *fileNodeBuilder) names(names []*ast.Name) []string {
	var out []string
	for _, n := range names {
		if n != nil {
			out = append(out, b.text(n))
		}
	}
	return out
}

func (b *fileNodeBuilder) class(c *ast.ClassDecl, conditional bool) ClassNode {
	mods := modifiers(c.Modifiers)
	body := b.body(c.Members, mods.readonly)
	node := ClassNode{
		Attributes:  b.attributes(c.Attrs),
		Extends:     b.nameText(c.Extends),
		Implements:  b.names(c.Implements),
		Traits:      body.traits,
		Properties:  body.properties,
		Methods:     body.methods,
		Constants:   body.constants,
		IsAbstract:  mods.abstract,
		IsFinal:     mods.final,
		IsReadonly:  mods.readonly,
		Conditional: conditional,
		EndLine:     b.endLine(c.Span),
		DocComment:  b.doc(c.Doc),
		TraitUseDoc: body.traitUseDoc,
		TraitRules:  body.traitRules,
	}
	if c.Name != nil {
		node.Name = b.text(c.Name)
		node.FullName = b.qualify(node.Name)
		node.StartLine, node.StartCol = b.position(c.Name.Start)
	}
	return node
}

func (b *fileNodeBuilder) nameText(n *ast.Name) string {
	if n == nil {
		return ""
	}
	return b.text(n)
}

func (b *fileNodeBuilder) anonymousClass(c *ast.ClassDecl) ClassNode {
	keyword := c.Start
	for _, group := range c.Attrs {
		keyword = group.End
	}
	for _, m := range c.Modifiers {
		keyword = m.End
	}
	keyword = b.skipTrivia(keyword)
	header := keyword + uint32(len("class"))
	if c.Args != nil {
		header = max(header, c.Args.End)
	}
	if c.Extends != nil {
		header = max(header, c.Extends.End)
	}
	for _, name := range c.Implements {
		header = max(header, name.End)
	}
	readonly := modifiers(c.Modifiers).readonly
	body := b.body(c.Members, readonly)
	line, col := b.position(keyword)
	bodyLine, bodyCol := b.position(b.skipTrivia(header))
	return ClassNode{
		Attributes:  b.attributes(c.Attrs),
		Name:        AnonymousClassName,
		FullName:    b.qualify(AnonymousClassName + "@" + strconv.Itoa(line+1) + "@" + b.hash()),
		Extends:     b.nameText(c.Extends),
		Implements:  b.names(c.Implements),
		Traits:      body.traits,
		Properties:  body.properties,
		Methods:     body.methods,
		Constants:   body.constants,
		IsReadonly:  readonly,
		StartLine:   line,
		StartCol:    col,
		EndLine:     b.endLine(c.Span),
		TraitUseDoc: body.traitUseDoc,
		TraitRules:  body.traitRules,
		BodyLine:    bodyLine,
		BodyCol:     bodyCol,
	}
}

func (b *fileNodeBuilder) hash() string {
	if b.fileHash == "" {
		h := fnv.New32a()
		h.Write([]byte(b.src))
		h.Write([]byte{'\n'})
		b.fileHash = strconv.FormatUint(uint64(h.Sum32()), 36)
	}
	return b.fileHash
}

func (b *fileNodeBuilder) iface(i *ast.InterfaceDecl) InterfaceNode {
	body := b.body(i.Members, false)
	node := InterfaceNode{
		Attributes: b.attributes(i.Attrs),
		Extends:    b.names(i.Extends),
		Methods:    body.methods,
		Properties: body.properties,
		Constants:  body.constants,
		EndLine:    b.endLine(i.Span),
		DocComment: b.doc(i.Doc),
	}
	if i.Name != nil {
		node.Name = b.text(i.Name)
		node.FullName = b.qualify(node.Name)
		node.StartLine, node.StartCol = b.position(i.Name.Start)
	}
	return node
}

func (b *fileNodeBuilder) trait(t *ast.TraitDecl) TraitNode {
	body := b.body(t.Members, false)
	node := TraitNode{
		Attributes:  b.attributes(t.Attrs),
		Constants:   body.constants,
		Properties:  body.properties,
		Methods:     body.methods,
		Traits:      body.traits,
		EndLine:     b.endLine(t.Span),
		DocComment:  b.doc(t.Doc),
		TraitUseDoc: body.traitUseDoc,
		TraitRules:  body.traitRules,
	}
	if t.Name != nil {
		node.Name = b.text(t.Name)
		node.FullName = b.qualify(node.Name)
		node.StartLine, node.StartCol = b.position(t.Name.Start)
	}
	return node
}

func (b *fileNodeBuilder) enum(e *ast.EnumDecl) EnumNode {
	body := b.body(e.Members, false)
	node := EnumNode{
		BackedType: b.typeText(e.BackingType),
		Cases:      body.cases,
		Methods:    body.methods,
		Properties: body.properties,
		Constants:  body.constants,
		Traits:     body.traits,
		TraitRules: body.traitRules,
		Implements: b.names(e.Implements),
		Attributes: b.attributes(e.Attrs),
		EndLine:    b.endLine(e.Span),
		DocComment: b.doc(e.Doc),
	}
	if e.Name != nil {
		node.Name = b.text(e.Name)
		node.FullName = b.qualify(node.Name)
		node.StartLine, node.StartCol = b.position(e.Name.Start)
	}
	return node
}

func (b *fileNodeBuilder) skipTrivia(i uint32) uint32 {
	src := b.src
	for int(i) < len(src) {
		switch {
		case src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r':
			i++
		case strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return uint32(len(src))
			}
			i += uint32(end) + 4
		case strings.HasPrefix(src[i:], "//") || src[i] == '#' && !strings.HasPrefix(src[i:], "#["):
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				return uint32(len(src))
			}
			i += uint32(end)
		default:
			return i
		}
	}
	return i
}

func (b *fileNodeBuilder) typeText(t ast.Type) string {
	if t == nil {
		return ""
	}
	s := t.Bounds()
	var out strings.Builder
	for i := s.Start; i < s.End; {
		next := b.skipTrivia(i)
		if next != i {
			i = next
			continue
		}
		out.WriteByte(b.src[i])
		i++
	}
	return out.String()
}

func (b *fileNodeBuilder) scalar(e ast.Expr) string {
	switch e.(type) {
	case *ast.Literal, *ast.ConstFetch:
		return b.text(e)
	}
	return ""
}

func (b *fileNodeBuilder) literalType(e ast.Expr, compact string) string {
	if e == nil {
		return ""
	}
	t := LiteralExprType(e, b.src)
	if t == compact {
		return compact
	}
	return strings.Clone(t)
}

func (b *fileNodeBuilder) compact(n ast.Node) string {
	if n == nil {
		return ""
	}
	s := n.Bounds()
	src := b.src[:s.End]
	var out strings.Builder
	prevWord := false
	for i := int(s.Start); i < len(src); {
		if next := int(b.skipTrivia(uint32(i))); next != i {
			i = min(next, len(src))
			continue
		}
		start := i
		c := src[i]
		switch {
		case c == '\'' || c == '"':
			i = len(src)
			if end := quotedEnd(src, start); end >= 0 {
				i = end + 1
			}
		case c == '$' || IsIdentByte(c):
			i++
			for i < len(src) && IsIdentByte(src[i]) {
				i++
			}
			if c >= '0' && c <= '9' {
				for i < len(src) && (IsIdentByte(src[i]) || src[i] == '.') {
					i++
				}
			}
		case strings.HasPrefix(src[i:], "<<<"):
			i = len(src)
		default:
			i++
		}
		word := c == '$' || c == '\'' || IsIdentByte(c)
		if prevWord && word {
			out.WriteByte(' ')
		}
		out.WriteString(src[start:i])
		prevWord = word
	}
	return out.String()
}
