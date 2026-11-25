package symbols

import (
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

type SymbolKind int

const (
	KindClass SymbolKind = iota
	KindInterface
	KindTrait
	KindEnum
	KindFunction
	KindMethod
	KindProperty
	KindConstant
	KindEnumCase
)

type SymbolSource int

const (
	SourceProject SymbolSource = iota
	SourceBuiltin
	SourceVendor
	// SourceGenerated marks classes a framework writes at build time, which the
	// index builds on demand from the class they're generated for.
	SourceGenerated
)

type Symbol struct {
	Name          string
	FQN           string
	Kind          SymbolKind
	Source        SymbolSource
	URI           string
	Range         protocol.Range
	Visibility    string
	SetVisibility string // PHP 8.4 asymmetric visibility (e.g. "private" in public private(set))
	IsStatic      bool
	IsAbstract    bool
	IsFinal       bool
	IsReadonly    bool
	Type          string
	DocComment    string
	ParentFQN     string
	Params        []ParamInfo
	ReturnType    string
	Children      []*Symbol
	Implements    []string
	Traits        []string
	traitRules    *traitRules
	// AliasOf is the trait method a member is an `as` alias of, under the
	// alias's own name in the class or trait whose use block makes it.
	AliasOf    *Symbol
	Extends    string
	BackedType string
	Value      string
	IsVirtual  bool
	// IsConditional marks a class declared inside a block, such as a
	// placeholder under `if (false)` or a polyfill under `if (!class_exists())`:
	// its members may not be those of the class code runs with.
	IsConditional bool
	Templates     []TemplateParam
	Supertypes    []Supertype
	TypeAliases   map[string]TypeAlias
	Hooks         []parser.PropertyHook // PHP 8.4 property hooks (get/set accessors)
	Attributes    []Attribute
}

func (s *Symbol) IsDeprecated() bool {
	return s.Attribute("Deprecated") != nil || (s.DocComment != "" && parser.ParseDocBlock(s.DocComment).Deprecated)
}

// Attribute is a PHP 8 attribute on a symbol.
type Attribute struct {
	Name string   // as written
	FQN  string   // what the name resolves to
	Args []string // each argument as written
}

// Attribute returns the attribute with this fully qualified name, if it has one.
func (s *Symbol) Attribute(fqn string) *Attribute {
	for i := range s.Attributes {
		if strings.EqualFold(s.Attributes[i].FQN, fqn) {
			return &s.Attributes[i]
		}
	}
	return nil
}

// TemplateParam represents a generic type parameter on a class, method or
// function: @template T of Model = User.
type TemplateParam struct {
	Name    string
	Bound   string
	Default string
}

// Supertype is a parent class, interface or trait together with the type
// arguments a class gives it in @extends, @implements or @use:
// `@extends Collection<int, User>` is {FQN: Collection, Args: [int, User]}.
// Args are PHPDoc types as written, with the declaring file's imports
// qualified; they may name the class's own templates.
type Supertype struct {
	FQN  string
	Args []string
}

// TypeAlias represents a structured PHPDoc alias declared on a symbol owner.
// Imported aliases keep Import populated and Type empty until resolved lazily.
type TypeAlias struct {
	Name   string
	Type   string
	Import *ImportedTypeAlias
}

// ImportedTypeAlias describes @phpstan-import-type/@psalm-import-type metadata.
type ImportedTypeAlias struct {
	FromFQN    string
	ImportedAs string
}

type ParamInfo struct {
	Name         string
	Type         string
	DefaultValue string
	IsVariadic   bool
	IsReference  bool
}

type Index struct {
	attributedSymbols atomic.Bool
	// generation counts changes to the index, so caches built from it can
	// tell when they're out of date.
	generation atomic.Uint64
	// narrowingNames counts, by lowercase name, the methods and functions
	// whose PHPDoc narrows a type (@phpstan-assert, @param-out, self-out).
	narrowingNames map[string]int
	// members caches member tables by class (see ClassMembersNamed).
	members              memberTables
	mu                   sync.RWMutex
	symbols              map[string]*Symbol
	shadowed             map[string][]*Symbol
	nameIndex            map[string][]string
	foldIndex            map[string][]string // lowercase FQN → FQNs of class-likes and functions, whose names PHP matches case-insensitively
	fileSymbols          map[string][]*Symbol
	namespaceIndex       map[string][]string
	optionalNamespaces   []string                   // lowercase prefixes of optional packages that aren't installed; see SetOptionalNamespaces
	callForwards         map[string][]string        // class FQN → types it forwards unknown calls to; see SetCallForwards
	scopeConventions     map[string]ScopeConvention // builder class FQN → how it finds its model's scopes; see SetScopeConventions
	inheritanceMap       map[string]string
	childrenMap          map[string][]string // lowercase parent FQN → direct subclass FQNs, the reverse of inheritanceMap
	implementsMap        map[string][]string // class → interfaces it implements
	reverseImplementsMap map[string][]string // interface → classes that implement it
	subInterfaceMap      map[string][]string // interface → interfaces that extend it
	traitMap             map[string][]string
	sortedNames          []string // sorted lowercase name keys for binary search
	sortedNamesDirty     bool     // rebuild flag
	sortedFQNs           []string // sorted FQN keys for binary search (SearchByFQNPrefix)
	sortedFQNsDirty      bool     // rebuild flag for sortedFQNs
	fileSource           map[string]indexedText
	ready                atomic.Bool // true once project + vendor indexing has completed at least once
	generator            atomic.Pointer[classGeneratorHolder]
	loader               atomic.Pointer[classLoaderHolder]
	generatedURIs        map[string]bool     // URIs of generated classes, which are not files on disk
	generatedFrom        map[string][]string // lowercase source FQN → URIs of the classes generated from it
	// templateFiles holds the URIs of templates (Blade views) in the
	// workspace. They declare no symbols, but references and rename read the
	// PHP in them.
	templateFiles map[string]struct{}
}

// Generation changes whenever symbols are added to or removed from the index.
// A cache of answers derived from the index keeps the generation it was built
// at and starts over when it differs.
func (idx *Index) Generation() uint64 {
	return idx.generation.Load()
}

// narrowsTypes reports whether a method or function's PHPDoc narrows the
// type of something passed to it or called on it.
func narrowsTypes(sym *Symbol) bool {
	if sym.Kind != KindMethod && sym.Kind != KindFunction || sym.DocComment == "" {
		return false
	}
	d := sym.DocComment
	return strings.Contains(d, "-assert") || strings.Contains(d, "param-out") ||
		strings.Contains(d, "self-out") || strings.Contains(d, "this-out")
}

// MayNarrow reports whether some method or function called name has PHPDoc
// that narrows types: @phpstan-assert and friends, @param-out, self-out.
// Code that narrows variables only needs to look at calls to those.
func (idx *Index) MayNarrow(name string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.narrowingNames[strings.ToLower(name)] > 0
}

func NewIndex() *Index {
	return &Index{
		symbols:              make(map[string]*Symbol),
		nameIndex:            make(map[string][]string),
		foldIndex:            make(map[string][]string),
		fileSymbols:          make(map[string][]*Symbol),
		namespaceIndex:       make(map[string][]string),
		inheritanceMap:       make(map[string]string),
		childrenMap:          make(map[string][]string),
		implementsMap:        make(map[string][]string),
		reverseImplementsMap: make(map[string][]string),
		subInterfaceMap:      make(map[string][]string),
		traitMap:             make(map[string][]string),
		fileSource:           make(map[string]indexedText),
		generatedURIs:        make(map[string]bool),
		generatedFrom:        make(map[string][]string),
	}
}

func (idx *Index) IndexFile(uri string, source string) {
	idx.IndexFileWithSource(uri, source, SourceProject)
}

// IndexIDEHelperFile indexes an IDE helper file (e.g. _ide_helper_models.php) by
// merging virtual members from @property/@method docblocks into existing class symbols.
// Classes that don't already exist in the index are indexed normally.
func (idx *Index) IndexIDEHelperFile(uri string, source string) {
	file := parser.ParseFile(source)
	if file == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove any previously indexed symbols from this URI (for re-indexing)
	idx.removeFileSymbols(uri)
	idx.fileSource[uri] = indexedText{text: source, ideHelper: true}
	if _, ok := idx.fileSymbols[uri]; !ok {
		idx.fileSymbols[uri] = nil
	}

	ns := file.Namespace
	resolve := func(name string) string {
		return resolveTypeName(name, ns, file.Uses)
	}

	for _, c := range file.Classes {
		fqn := buildFQN(ns, c.Name)
		existing := idx.symbols[fqn]
		if existing != nil {
			// Class already exists — only inject virtual members from this file's docblock
			if c.DocComment != "" {
				doc := parser.ParseDocBlock(c.DocComment)
				if doc != nil {
					indexTypeAliases(existing, doc, resolve)
					for _, prop := range doc.Properties {
						if prop.Name == "" {
							continue
						}
						propName := "$" + prop.Name
						if idx.symbols[fqn+"::"+propName] != nil {
							continue
						}
						ps := &Symbol{
							Name:       propName,
							FQN:        fqn + "::" + propName,
							Kind:       KindProperty,
							URI:        uri,
							Visibility: "public",
							Type:       resolve(prop.Type),
							ParentFQN:  fqn,
							IsVirtual:  true,
							DocComment: prop.Description,
						}
						existing.Children = append(existing.Children, ps)
						idx.addSymbolWithSource(uri, ps, SourceProject)
					}
					for _, method := range doc.Methods {
						if method.Name == "" {
							continue
						}
						if idx.symbols[fqn+"::"+method.Name] != nil {
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
							DocComment: method.Description,
						}
						if method.Params != "" {
							ms.Params = parseDocMethodParams(method.Params, resolve)
						}
						existing.Children = append(existing.Children, ms)
						idx.addSymbolWithSource(uri, ms, SourceProject)
					}
				}
			}
		} else {
			// Class doesn't exist yet — index it normally
			sym := &Symbol{Name: c.Name, FQN: fqn, Kind: KindClass, URI: uri, DocComment: qualifyDocTypes(c.DocComment, file.Uses),
				IsAbstract: c.IsAbstract, IsFinal: c.IsFinal, IsReadonly: c.IsReadonly, IsConditional: c.Conditional,
				Implements: resolveNames(c.Implements, resolve), Traits: resolveNames(c.Traits, resolve), traitRules: newTraitRules(fqn, c.TraitRules, resolve),
				Range: symRange(c.StartLine, c.StartCol, len(c.Name))}
			if c.Extends != "" {
				sym.Extends = resolve(c.Extends)
			}
			idx.addSymbolWithSource(uri, sym, SourceProject)
			idx.linkTypeLocked(sym)
			for _, prop := range c.Properties {
				ps := &Symbol{Name: prop.Name, FQN: fqn + "::" + prop.Name, Kind: KindProperty, URI: uri,
					Visibility: prop.Visibility, SetVisibility: prop.SetVisibility,
					IsStatic: prop.IsStatic, Type: resolve(prop.Type.Name),
					DocComment: qualifyDocTypes(prop.DocComment, file.Uses), ParentFQN: fqn,
					Hooks: append([]parser.PropertyHook(nil), prop.Hooks...),
					Range: symRange(prop.StartLine, prop.StartCol, len(prop.Name))}
				sym.Children = append(sym.Children, ps)
				idx.addSymbolWithSource(uri, ps, SourceProject)
			}
			idx.indexMethods(uri, sym, fqn, c.Methods, SourceProject, resolve, file.Uses)
			idx.indexVirtualMembers(uri, sym, SourceProject, resolve)
		}
	}
}

type indexedText struct {
	text      string
	from      SymbolSource
	ideHelper bool
}

func (idx *Index) IndexFileWithSource(uri string, source string, src SymbolSource) {
	indexed := indexedText{text: source, from: src}
	idx.mu.RLock()
	previous, indexedBefore := idx.fileSource[uri]
	idx.mu.RUnlock()
	if indexedBefore && previous == indexed {
		return
	}
	file := parser.ParseFile(source)
	if file == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	foreign := idx.foreignMembersLocked(uri)
	idx.removeFileSymbols(uri)
	idx.fileSource[uri] = indexed
	idx.indexParsedLocked(uri, file, src)
	idx.reattachForeignMembersLocked(foreign)
}

// foreignMembersLocked returns, by class, the members other files added to
// the classes uri declares, such as an IDE helper file's @property columns
// on a model. Re-indexing uri replaces those classes, so the members are
// attached again to the new ones afterwards.
func (idx *Index) foreignMembersLocked(uri string) map[string][]*Symbol {
	var out map[string][]*Symbol
	for _, sym := range idx.fileSymbols[uri] {
		if sym.Kind.IsMember() {
			continue
		}
		for _, child := range sym.Children {
			if child.URI != uri && idx.symbols[child.FQN] == child {
				if out == nil {
					out = map[string][]*Symbol{}
				}
				out[sym.FQN] = append(out[sym.FQN], child)
			}
		}
	}
	return out
}

// reattachForeignMembersLocked gives each class back the members other files
// added to it, unless the class now declares a member of that name itself.
func (idx *Index) reattachForeignMembersLocked(foreign map[string][]*Symbol) {
	for fqn, members := range foreign {
		parent := idx.symbols[fqn]
		if parent == nil {
			continue
		}
		for _, m := range members {
			if idx.symbols[m.FQN] != m {
				continue
			}
			declared := false
			for _, c := range parent.Children {
				if c.Kind == m.Kind && c.Name == m.Name {
					declared = true
					break
				}
			}
			if !declared {
				parent.Children = append(parent.Children, m)
			}
		}
	}
}

// indexParsedLocked adds the symbols a parsed file declares. The caller holds
// the write lock and has removed the file's previous symbols.
func (idx *Index) indexParsedLocked(uri string, file *parser.FileNode, src SymbolSource) {
	// Ensure the file is tracked even if it has no symbol declarations
	if _, ok := idx.fileSymbols[uri]; !ok {
		idx.fileSymbols[uri] = nil
	}
	ns := file.Namespace

	resolve := func(name string) string {
		return resolveTypeName(name, ns, file.Uses)
	}

	// Anonymous classes are indexed like declared ones, under their file-unique
	// FullName, so their members and parents resolve; addSymbol keeps them out
	// of name search.
	classes := file.Classes
	if len(file.AnonymousClasses) > 0 {
		classes = append(append(make([]parser.ClassNode, 0, len(file.Classes)+len(file.AnonymousClasses)), file.Classes...), file.AnonymousClasses...)
	}
	for _, c := range classes {
		fqn := buildFQN(ns, c.Name)
		if c.Name == parser.AnonymousClassName {
			fqn = c.FullName
		}
		sym := &Symbol{Name: c.Name, FQN: fqn, Kind: KindClass, URI: uri, DocComment: qualifyDocTypes(c.DocComment, file.Uses),
			IsAbstract: c.IsAbstract, IsFinal: c.IsFinal, IsReadonly: c.IsReadonly, IsConditional: c.Conditional,
			Implements: resolveNames(c.Implements, resolve), Traits: resolveNames(c.Traits, resolve), traitRules: newTraitRules(fqn, c.TraitRules, resolve),
			Attributes: indexAttributes(c.Attributes, resolve),
			Range:      symRange(c.StartLine, c.StartCol, len(c.Name))}
		previous := idx.symbols[fqn]
		if c.Extends != "" {
			sym.Extends = resolve(c.Extends)
		}
		idx.addSymbolWithSource(uri, sym, src)
		idx.indexProperties(uri, sym, fqn, c.Properties, src, resolve, file.Uses)
		idx.indexMethods(uri, sym, fqn, c.Methods, src, resolve, file.Uses)
		idx.indexConstants(uri, sym, fqn, c.Constants, src, resolve)
		idx.indexVirtualMembers(uri, sym, src, resolve)
		sym.Supertypes = append(sym.Supertypes, traitUseSupertypes(c.TraitUseDoc, file.Uses, resolve)...)
		if src == SourceBuiltin {
			mergeEarlierLayer(sym, previous)
		}
		idx.linkTypeLocked(sym)
	}

	for _, iface := range file.Interfaces {
		fqn := buildFQN(ns, iface.Name)
		sym := &Symbol{Name: iface.Name, FQN: fqn, Kind: KindInterface, URI: uri, DocComment: qualifyDocTypes(iface.DocComment, file.Uses),
			Implements: resolveNames(iface.Extends, resolve), Attributes: indexAttributes(iface.Attributes, resolve),
			Range: symRange(iface.StartLine, iface.StartCol, len(iface.Name))}
		idx.addSymbolWithSource(uri, sym, src)
		idx.linkTypeLocked(sym)
		idx.indexProperties(uri, sym, fqn, iface.Properties, src, resolve, file.Uses)
		idx.indexMethods(uri, sym, fqn, iface.Methods, src, resolve, file.Uses)
		idx.indexConstants(uri, sym, fqn, iface.Constants, src, resolve)
		idx.indexVirtualMembers(uri, sym, src, resolve)
	}

	for _, tr := range file.Traits {
		fqn := buildFQN(ns, tr.Name)
		sym := &Symbol{Name: tr.Name, FQN: fqn, Kind: KindTrait, URI: uri, DocComment: qualifyDocTypes(tr.DocComment, file.Uses),
			Traits: resolveNames(tr.Traits, resolve), traitRules: newTraitRules(fqn, tr.TraitRules, resolve),
			Range: symRange(tr.StartLine, tr.StartCol, len(tr.Name))}
		idx.addSymbolWithSource(uri, sym, src)
		idx.linkTypeLocked(sym)
		idx.indexProperties(uri, sym, fqn, tr.Properties, src, resolve, file.Uses)
		idx.indexMethods(uri, sym, fqn, tr.Methods, src, resolve, file.Uses)
		idx.indexConstants(uri, sym, fqn, tr.Constants, src, resolve)
		idx.indexVirtualMembers(uri, sym, src, resolve)
		sym.Supertypes = append(sym.Supertypes, traitUseSupertypes(tr.TraitUseDoc, file.Uses, resolve)...)
	}

	for _, en := range file.Enums {
		fqn := buildFQN(ns, en.Name)
		sym := &Symbol{Name: en.Name, FQN: fqn, Kind: KindEnum, URI: uri, DocComment: qualifyDocTypes(en.DocComment, file.Uses),
			BackedType: en.BackedType, Implements: resolveNames(en.Implements, resolve), Traits: resolveNames(en.Traits, resolve),
			traitRules: newTraitRules(fqn, en.TraitRules, resolve),
			Attributes: indexAttributes(en.Attributes, resolve),
			Range:      symRange(en.StartLine, en.StartCol, len(en.Name))}
		idx.addSymbolWithSource(uri, sym, src)
		idx.linkTypeLocked(sym)
		for _, ec := range en.Cases {
			cs := &Symbol{Name: ec.Name, FQN: fqn + "::" + ec.Name, Kind: KindEnumCase, URI: uri, ParentFQN: fqn, Value: ec.Value,
				DocComment: ec.DocComment, Attributes: indexAttributes(ec.Attributes, resolve),
				Range: symRange(ec.StartLine, ec.StartCol, len(ec.Name))}
			sym.Children = append(sym.Children, cs)
			idx.addSymbolWithSource(uri, cs, src)
		}
		idx.indexProperties(uri, sym, fqn, en.Properties, src, resolve, file.Uses)
		idx.indexMethods(uri, sym, fqn, en.Methods, src, resolve, file.Uses)
		idx.indexConstants(uri, sym, fqn, en.Constants, src, resolve)
		idx.indexEnumMembers(uri, sym, src)
	}

	idx.indexConstants(uri, nil, ns, file.Constants, src, resolve)

	for _, fn := range file.Functions {
		fqn := buildFQN(ns, fn.Name)
		sym := &Symbol{Name: fn.Name, FQN: fqn, Kind: KindFunction, URI: uri, ReturnType: resolve(fn.ReturnType.Name),
			Attributes: indexAttributes(fn.Attributes, resolve),
			DocComment: qualifyDocTypes(fn.DocComment, file.Uses), Range: symRange(fn.StartLine, fn.StartCol, len(fn.Name))}
		sym.Templates = docTemplates(sym.DocComment, resolve)
		for _, p := range fn.Params {
			sym.Params = append(sym.Params, ParamInfo{Name: p.Name, Type: resolve(p.Type.Name), DefaultValue: p.DefaultValue, IsVariadic: p.IsVariadic, IsReference: p.IsReference})
		}
		idx.addSymbolWithSource(uri, sym, src)
	}
}

func (idx *Index) addSymbol(uri string, sym *Symbol) {
	idx.generation.Add(1)
	if narrowsTypes(sym) {
		if idx.narrowingNames == nil {
			idx.narrowingNames = make(map[string]int)
		}
		idx.narrowingNames[strings.ToLower(sym.Name)]++
	}
	if sym.Source != SourceGenerated && !sym.Kind.IsMember() {
		// A real declaration replaces a class generated under the same name.
		idx.dropGeneratedLocked(sym.FQN)
	}
	if sym.isGlobal() {
		idx.shadowLocked(sym)
	}
	idx.storeSymbolLocked(sym, pendingRemovals{})
	if len(sym.Attributes) > 0 {
		idx.attributedSymbols.Store(true)
	}
	idx.fileSymbols[uri] = append(idx.fileSymbols[uri], sym)
}

func (idx *Index) storeSymbolLocked(sym *Symbol, pending pendingRemovals) {
	if _, ok := idx.symbols[sym.FQN]; !ok {
		idx.indexNamesLocked(sym, pending)
	}
	idx.symbols[sym.FQN] = sym
}

func (idx *Index) indexNamesLocked(sym *Symbol, pending pendingRemovals) {
	if sym.Kind == KindClass && sym.Name == parser.AnonymousClassName {
		// Found by its FQN only: no one can write its name.
		return
	}
	idx.sortedNamesDirty = true
	idx.sortedFQNsDirty = true
	if !pending.byName.take(sym.Name, sym.FQN) {
		idx.nameIndex[sym.Name] = append(idx.nameIndex[sym.Name], sym.FQN)
	}
	if sym.Kind.IsMember() {
		return
	}
	if ns := namespaceForFQN(sym.FQN); !pending.byNamespace.take(ns, sym.FQN) {
		idx.namespaceIndex[ns] = append(idx.namespaceIndex[ns], sym.FQN)
	}
	if key := strings.ToLower(sym.FQN); !pending.byFold.take(key, sym.FQN) {
		idx.foldIndex[key] = append(idx.foldIndex[key], sym.FQN)
	}
}

// LookupFold finds a class-like or function the way PHP does, ignoring case:
// `new stdclass`, `PclZip` declared as `pclzip`, `floatVal()`. An exact match
// wins. match, when not nil, must accept the symbol (for example to want a
// class and not a function of the same name).
func (idx *Index) LookupFold(fqn string, match func(*Symbol) bool) *Symbol {
	idx.ensureClass(fqn)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if sym := idx.symbols[fqn]; sym != nil && (match == nil || match(sym)) {
		return sym
	}
	for _, candidate := range idx.foldIndex[strings.ToLower(fqn)] {
		if sym := idx.symbols[candidate]; sym != nil && (match == nil || match(sym)) {
			return sym
		}
	}
	return nil
}

func (idx *Index) addSymbolWithSource(uri string, sym *Symbol, src SymbolSource) {
	sym.Source = src
	idx.addSymbol(uri, sym)
}

func (idx *Index) removeFileSymbols(uri string) {
	idx.generation.Add(1)
	var pending pendingRemovals
	for _, sym := range idx.fileSymbols[uri] {
		// A member this file added to a class another file declares (an IDE
		// helper's @property) leaves that class's members with it.
		if sym.Kind.IsMember() && sym.ParentFQN != "" {
			if parent := idx.symbols[sym.ParentFQN]; parent != nil && parent.URI != uri {
				parent.Children = withoutSymbol(parent.Children, sym)
			}
		}
		if narrowsTypes(sym) {
			key := strings.ToLower(sym.Name)
			if idx.narrowingNames[key] <= 1 {
				delete(idx.narrowingNames, key)
			} else {
				idx.narrowingNames[key]--
			}
		}
		if !sym.Kind.IsMember() {
			// Classes generated from this one are rebuilt on their next lookup.
			idx.dropGeneratedFromLocked(sym.FQN)
		}
		if idx.symbols[sym.FQN] != sym {
			idx.unshadowLocked(sym)
			continue
		}
		if sym.isGlobal() && idx.promoteShadowedLocked(sym, pending) {
			continue
		}
		delete(idx.symbols, sym.FQN)
		idx.sortedNamesDirty = true
		idx.sortedFQNsDirty = true
		pending.byName.add(sym.Name, sym.FQN)
		if !sym.Kind.IsMember() {
			pending.byNamespace.add(namespaceForFQN(sym.FQN), sym.FQN)
			pending.byFold.add(strings.ToLower(sym.FQN), sym.FQN)
		}
		idx.unlinkTypeLocked(sym)
	}
	pending.byName.applyTo(idx.nameIndex)
	pending.byNamespace.applyTo(idx.namespaceIndex)
	pending.byFold.applyTo(idx.foldIndex)
	delete(idx.fileSymbols, uri)
	delete(idx.fileSource, uri)
	delete(idx.generatedURIs, uri)
}

func (idx *Index) shadowLocked(sym *Symbol) {
	prev := idx.symbols[sym.FQN]
	if prev == nil || prev == sym || !prev.isGlobal() || prev.Source == SourceGenerated || sym.Source == SourceGenerated ||
		(prev.Source == SourceBuiltin && sym.Source == SourceBuiltin) {
		return
	}
	if idx.shadowed == nil {
		idx.shadowed = make(map[string][]*Symbol)
	}
	idx.shadowed[sym.FQN] = append(idx.shadowed[sym.FQN], prev)
	idx.unlinkTypeLocked(prev)
}

func (idx *Index) unshadowLocked(sym *Symbol) {
	defs, ok := idx.shadowed[sym.FQN]
	if !ok {
		return
	}
	if defs = withoutSymbol(defs, sym); len(defs) > 0 {
		idx.shadowed[sym.FQN] = defs
	} else {
		delete(idx.shadowed, sym.FQN)
	}
}

func (idx *Index) promoteShadowedLocked(removed *Symbol, pending pendingRemovals) bool {
	defs := idx.shadowed[removed.FQN]
	if len(defs) == 0 {
		return false
	}
	promoted := defs[len(defs)-1]
	idx.unshadowLocked(promoted)
	idx.unlinkTypeLocked(removed)
	idx.storeSymbolLocked(promoted, pending)
	for _, child := range promoted.Children {
		idx.storeSymbolLocked(child, pending)
	}
	idx.linkTypeLocked(promoted)
	return true
}

func (idx *Index) linkTypeLocked(sym *Symbol) {
	fqn := sym.FQN
	if sym.Extends != "" {
		idx.setParentLocked(fqn, sym.Extends)
	}
	for _, parent := range sym.Implements {
		idx.implementsMap[fqn] = appendUnique(idx.implementsMap[fqn], parent)
		if sym.Kind == KindInterface {
			idx.subInterfaceMap[parent] = appendUnique(idx.subInterfaceMap[parent], fqn)
		} else {
			idx.reverseImplementsMap[parent] = appendUnique(idx.reverseImplementsMap[parent], fqn)
		}
	}
	for _, tr := range sym.Traits {
		idx.traitMap[fqn] = appendUnique(idx.traitMap[fqn], tr)
	}
}

func (idx *Index) unlinkTypeLocked(sym *Symbol) {
	fqn := sym.FQN
	for _, parent := range idx.implementsMap[fqn] {
		if sym.Kind == KindInterface {
			idx.subInterfaceMap[parent] = removeFromSlice(idx.subInterfaceMap[parent], fqn)
		} else {
			idx.reverseImplementsMap[parent] = removeFromSlice(idx.reverseImplementsMap[parent], fqn)
		}
	}
	delete(idx.implementsMap, fqn)
	idx.clearParentLocked(fqn)
	delete(idx.traitMap, fqn)
}

func resolveNames(names []string, resolve func(string) string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, resolve(name))
	}
	return out
}

func (idx *Index) clearParentLocked(fqn string) {
	parent, ok := idx.inheritanceMap[fqn]
	if !ok {
		return
	}
	key := strings.ToLower(parent)
	if children := removeFromSlice(idx.childrenMap[key], fqn); len(children) > 0 {
		idx.childrenMap[key] = children
	} else {
		delete(idx.childrenMap, key)
	}
	delete(idx.inheritanceMap, fqn)
}

// withoutSymbol returns a copy of syms without sym; readers may hold the
// original slice.
func withoutSymbol(syms []*Symbol, sym *Symbol) []*Symbol {
	for i, s := range syms {
		if s == sym {
			out := make([]*Symbol, 0, len(syms)-1)
			out = append(out, syms[:i]...)
			return append(out, syms[i+1:]...)
		}
	}
	return syms
}

func namespaceForFQN(fqn string) string {
	if i := strings.LastIndex(fqn, "\\"); i >= 0 {
		return fqn[:i]
	}
	return ""
}

func (idx *Index) indexMethods(uri string, parent *Symbol, parentFQN string, methods []parser.MethodNode, src SymbolSource, resolve func(string) string, uses []parser.UseNode) {
	for _, m := range methods {
		ms := &Symbol{
			Name:       m.Name,
			FQN:        parentFQN + "::" + m.Name,
			Kind:       KindMethod,
			Attributes: indexAttributes(m.Attributes, resolve),
			URI:        uri,
			Visibility: m.Visibility,
			IsStatic:   m.IsStatic,
			IsAbstract: m.IsAbstract,
			IsFinal:    m.IsFinal,
			ReturnType: resolve(m.ReturnType.Name),
			DocComment: qualifyDocTypes(m.DocComment, uses),
			ParentFQN:  parentFQN,
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
		idx.addSymbolWithSource(uri, ms, src)
	}
}

func (idx *Index) indexProperties(uri string, parent *Symbol, parentFQN string, props []parser.PropertyNode, src SymbolSource, resolve func(string) string, uses []parser.UseNode) {
	for _, prop := range props {
		ps := &Symbol{Name: prop.Name, FQN: parentFQN + "::" + prop.Name, Kind: KindProperty, URI: uri,
			Visibility: prop.Visibility, SetVisibility: prop.SetVisibility,
			IsStatic: prop.IsStatic, IsReadonly: prop.IsReadonly, Type: resolve(prop.Type.Name),
			DocComment: qualifyDocTypes(prop.DocComment, uses), ParentFQN: parentFQN, Attributes: indexAttributes(prop.Attributes, resolve),
			Hooks: append([]parser.PropertyHook(nil), prop.Hooks...),
			Range: symRange(prop.StartLine, prop.StartCol, len(prop.Name))}
		parent.Children = append(parent.Children, ps)
		idx.addSymbolWithSource(uri, ps, src)
	}
}

// indexEnumMembers adds what PHP gives every enum: `name` and `cases()`, and for
// backed enums `value`, `from()` and `tryFrom()`. They point at the enum itself.
func (idx *Index) indexEnumMembers(uri string, enum *Symbol, src SymbolSource) {
	add := func(member *Symbol) {
		member.FQN = enum.FQN + "::" + member.Name
		member.URI = uri
		member.ParentFQN = enum.FQN
		member.Visibility = "public"
		member.Range = enum.Range
		enum.Children = append(enum.Children, member)
		idx.addSymbolWithSource(uri, member, src)
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

// mergeEarlierLayer keeps what an earlier stub layer declared when a later one
// only adds members to the same class, as version deltas do.
func mergeEarlierLayer(sym, previous *Symbol) {
	if previous == nil || previous.URI == sym.URI || previous.Kind != sym.Kind {
		return
	}
	declared := make(map[string]bool, len(sym.Children))
	for _, child := range sym.Children {
		declared[child.Name] = true
	}
	for _, child := range previous.Children {
		if !declared[child.Name] {
			sym.Children = append(sym.Children, child)
		}
	}
	if sym.DocComment == "" {
		sym.DocComment = previous.DocComment
	}
	if len(sym.Templates) == 0 {
		sym.Templates = previous.Templates
	}
	if len(sym.Supertypes) == 0 {
		sym.Supertypes = previous.Supertypes
	}
	if sym.Extends == "" {
		sym.Extends = previous.Extends
	}
	for _, impl := range previous.Implements {
		sym.Implements = appendUnique(sym.Implements, impl)
	}
	for _, tr := range previous.Traits {
		sym.Traits = appendUnique(sym.Traits, tr)
	}
	if sym.traitRules == nil {
		sym.traitRules = previous.traitRules
	}
}

func indexAttributes(attrs []parser.AttributeDef, resolve func(string) string) []Attribute {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]Attribute, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, Attribute{Name: attr.Name, FQN: resolve(attr.Name), Args: attr.Args})
	}
	return out
}

// indexVirtualMembers parses @property/@method docblock tags on a class-level
// symbol and injects them as virtual children so they appear in completion/hover.
func (idx *Index) indexVirtualMembers(uri string, parent *Symbol, src SymbolSource, resolve func(string) string) {
	if parent.DocComment == "" {
		return
	}
	doc := parser.ParseDocBlock(parent.DocComment)
	if doc == nil {
		return
	}
	fqn := parent.FQN

	indexTypeAliases(parent, doc, resolve)

	// Populate template parameters from @template tags
	parent.Templates = templateParams(doc, resolve)
	parent.Supertypes = supertypes(doc, resolve)

	for _, prop := range doc.Properties {
		if prop.Name == "" {
			continue
		}
		propName := "$" + prop.Name
		// Skip if a real member with this name already exists
		if idx.symbols[fqn+"::"+propName] != nil {
			continue
		}
		ps := &Symbol{
			Name:       propName,
			FQN:        fqn + "::" + propName,
			Kind:       KindProperty,
			URI:        uri,
			Visibility: "public",
			Type:       resolve(prop.Type),
			ParentFQN:  fqn,
			IsVirtual:  true,
			DocComment: prop.Description,
		}
		parent.Children = append(parent.Children, ps)
		idx.addSymbolWithSource(uri, ps, src)
	}

	for _, method := range doc.Methods {
		if method.Name == "" {
			continue
		}
		if idx.symbols[fqn+"::"+method.Name] != nil {
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
		// Parse params string into ParamInfo slice
		if method.Params != "" {
			ms.Params = parseDocMethodParams(method.Params, resolve)
		}
		parent.Children = append(parent.Children, ms)
		idx.addSymbolWithSource(uri, ms, src)
	}
}

// parseDocMethodParams parses a comma-separated param string like "string $name, int $age = 0"
// into a slice of ParamInfo.
func parseDocMethodParams(raw string, resolve func(string) string) []ParamInfo {
	var params []ParamInfo
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		p := ParamInfo{}
		fields := strings.Fields(part)
		for _, f := range fields {
			if f == "=" {
				break
			}
			if strings.HasPrefix(f, "$") {
				p.Name = f
			} else if strings.HasPrefix(f, "...") {
				p.IsVariadic = true
				rest := strings.TrimPrefix(f, "...")
				if strings.HasPrefix(rest, "$") {
					p.Name = rest
				} else if rest != "" {
					p.Type = resolve(rest)
				}
			} else if p.Name == "" && p.Type == "" {
				p.Type = resolve(f)
			}
		}
		params = append(params, p)
	}
	return params
}

func (idx *Index) Lookup(fqn string) *Symbol {
	idx.mu.RLock()
	sym := idx.symbols[fqn]
	idx.mu.RUnlock()
	if sym == nil && idx.ensureClass(fqn) {
		idx.mu.RLock()
		sym = idx.symbols[fqn]
		idx.mu.RUnlock()
	}
	return sym
}

// LookupTypeAlias returns the alias declared or imported on the given owner symbol.
func (idx *Index) LookupTypeAlias(ownerFQN, aliasName string) (TypeAlias, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	owner := idx.symbols[ownerFQN]
	if owner == nil || owner.TypeAliases == nil {
		return TypeAlias{}, false
	}
	alias, ok := owner.TypeAliases[aliasName]
	return alias, ok
}

func (idx *Index) LookupByName(name string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var results []*Symbol
	for _, fqn := range idx.nameIndex[name] {
		if sym, ok := idx.symbols[fqn]; ok {
			results = append(results, sym)
		}
	}
	return results
}

func (idx *Index) ensureSortedNames() {
	idx.mu.RLock()
	needRebuild := idx.sortedNamesDirty || len(idx.sortedNames) == 0
	idx.mu.RUnlock()
	if !needRebuild {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if !idx.sortedNamesDirty && len(idx.sortedNames) > 0 {
		return
	}
	idx.sortedNames = make([]string, 0, len(idx.nameIndex))
	for name := range idx.nameIndex {
		idx.sortedNames = append(idx.sortedNames, name)
	}
	sort.Slice(idx.sortedNames, func(i, j int) bool {
		return strings.ToLower(idx.sortedNames[i]) < strings.ToLower(idx.sortedNames[j])
	})
	idx.sortedNamesDirty = false
}

// FirstDeclaredByName returns the declared symbols (not builtins, and with a
// file) whose kind keep accepts, in case-insensitive name order, stopping at
// the first name that brings the count to limit or beyond. Every symbol of
// that last name is included, so which ones are returned doesn't depend on
// the order they were indexed in.
func (idx *Index) FirstDeclaredByName(limit int, keep func(SymbolKind) bool) []*Symbol {
	if limit <= 0 {
		return nil
	}
	idx.ensureSortedNames()

	idx.mu.RLock()
	defer idx.mu.RUnlock()
	results := make([]*Symbol, 0, limit)
	for _, name := range idx.sortedNames {
		for _, fqn := range idx.nameIndex[name] {
			sym, ok := idx.symbols[fqn]
			if !ok || IsBuiltin(sym) || sym.URI == "" || !keep(sym.Kind) {
				continue
			}
			results = append(results, sym)
		}
		if len(results) >= limit {
			break
		}
	}
	return results
}

func (idx *Index) SearchByPrefix(prefix string) []*Symbol {
	// Empty prefix: return all symbols without touching the sorted slice.
	if prefix == "" {
		idx.mu.RLock()
		defer idx.mu.RUnlock()
		var results []*Symbol
		for _, fqns := range idx.nameIndex {
			for _, fqn := range fqns {
				if sym, ok := idx.symbols[fqn]; ok {
					results = append(results, sym)
				}
			}
		}
		return results
	}

	idx.ensureSortedNames()

	// Binary search for the first matching prefix under a read lock.
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	lp := strings.ToLower(prefix)
	start := sort.Search(len(idx.sortedNames), func(i int) bool {
		return strings.ToLower(idx.sortedNames[i]) >= lp
	})

	var results []*Symbol
	for i := start; i < len(idx.sortedNames); i++ {
		name := idx.sortedNames[i]
		if !strings.HasPrefix(strings.ToLower(name), lp) {
			break
		}
		for _, fqn := range idx.nameIndex[name] {
			if sym, ok := idx.symbols[fqn]; ok {
				results = append(results, sym)
			}
		}
	}
	return results
}

// rebuildSortedFQNs rebuilds the sorted FQN slice used by SearchByFQNPrefix.
// Must be called with the write lock held.
func (idx *Index) rebuildSortedFQNs() {
	idx.sortedFQNs = make([]string, 0, len(idx.symbols))
	for fqn, sym := range idx.symbols {
		// Only top-level symbols are needed for FQN-prefix search
		if sym.Kind.IsMember() {
			continue
		}
		// Never store a symbol with an empty short name (M1 guard), nor an
		// anonymous class, which no one can write.
		if sym.Name == "" || sym.Name == parser.AnonymousClassName {
			continue
		}
		idx.sortedFQNs = append(idx.sortedFQNs, fqn)
	}
	sort.Slice(idx.sortedFQNs, func(i, j int) bool {
		return strings.ToLower(idx.sortedFQNs[i]) < strings.ToLower(idx.sortedFQNs[j])
	})
	idx.sortedFQNsDirty = false
}

// SearchByFQNPrefix returns symbols whose FQN starts with the given prefix,
// plus unique namespace segments at the next level for progressive completion.
// For example, prefix "Illuminate\" returns both direct symbols in that namespace
// and child namespace names like "Foundation", "Http", "Support", etc.
// Uses binary search over a sorted FQN slice — O(log N + K) where K is the
// number of matching symbols.
func (idx *Index) SearchByFQNPrefix(prefix string) ([]*Symbol, []string) {
	// Ensure the sorted FQN slice is up-to-date under a write lock if needed,
	// then perform the binary search under a read lock.
	idx.mu.RLock()
	needRebuild := idx.sortedFQNsDirty || len(idx.sortedFQNs) == 0
	idx.mu.RUnlock()

	if needRebuild {
		idx.mu.Lock()
		// Re-check under write lock to avoid duplicate work when two goroutines
		// both observed dirty and one completes the rebuild before the other.
		if idx.sortedFQNsDirty || len(idx.sortedFQNs) == 0 {
			idx.rebuildSortedFQNs()
		}
		idx.mu.Unlock()
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	lp := strings.ToLower(prefix)
	// Find the first FQN >= prefix (case-insensitive)
	start := sort.Search(len(idx.sortedFQNs), func(i int) bool {
		return strings.ToLower(idx.sortedFQNs[i]) >= lp
	})

	var syms []*Symbol
	nsSeen := make(map[string]bool)
	var nsSegments []string

	for i := start; i < len(idx.sortedFQNs); i++ {
		fqn := idx.sortedFQNs[i]
		if !strings.HasPrefix(strings.ToLower(fqn), lp) {
			break
		}
		sym, ok := idx.symbols[fqn]
		if !ok || sym.Name == "" {
			// M1: skip empty-name symbols
			continue
		}
		// Compute the suffix after the prefix (using original case for slicing)
		// Account for case-insensitive prefix match: find actual offset
		rest := fqn[len(prefix):]
		if sepIdx := strings.Index(rest, "\\"); sepIdx >= 0 {
			// This symbol is in a deeper namespace — extract the next segment
			seg := rest[:sepIdx]
			if seg != "" && !nsSeen[seg] {
				nsSeen[seg] = true
				nsSegments = append(nsSegments, seg)
			}
		} else {
			// Direct member of this namespace prefix
			syms = append(syms, sym)
		}
	}
	return syms, nsSegments
}

// WarmSearchIndexes eagerly builds the sorted lookup slices used by prefix and
// FQN-prefix search so serving reads can stay on the read lock path only.
func (idx *Index) WarmSearchIndexes() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.sortedNamesDirty || len(idx.sortedNames) == 0 {
		idx.sortedNames = make([]string, 0, len(idx.nameIndex))
		for name := range idx.nameIndex {
			idx.sortedNames = append(idx.sortedNames, name)
		}
		sort.Slice(idx.sortedNames, func(i, j int) bool {
			return strings.ToLower(idx.sortedNames[i]) < strings.ToLower(idx.sortedNames[j])
		})
		idx.sortedNamesDirty = false
	}
	if idx.sortedFQNsDirty || len(idx.sortedFQNs) == 0 {
		idx.rebuildSortedFQNs()
	}
}

func (idx *Index) GetFileSymbols(uri string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.fileSymbols[uri]
}

func (idx *Index) GetClassMembers(classFQN string) []*Symbol {
	idx.ensureClass(classFQN)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.getClassMembersRecursive(classFQN, make(map[string]bool))
}

func (idx *Index) TraitMembers(fqn string) []*Symbol {
	idx.ensureClass(fqn)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.appendTraitMembersLocked(nil, idx.traitMap[fqn], idx.traitRulesLocked(fqn), map[string]bool{fqn: true})
}

func (idx *Index) appendTraitMembersLocked(members []*Symbol, traits []string, rules *traitRules, visited map[string]bool) []*Symbol {
	start := len(members)
	var aliases []*Symbol
	for _, tr := range traits {
		if rules == nil {
			members = append(members, idx.getClassMembersRecursive(tr, visited)...)
			continue
		}
		for _, member := range idx.getClassMembersRecursive(tr, visited) {
			kept, renamed := rules.adapt(tr, member)
			aliases = append(aliases, renamed...)
			if kept != nil {
				members = append(members, kept)
			}
		}
	}
	members = append(members, aliases...)
	return members[:start+len(withoutAbstractsImplemented(members[start:]))]
}

func withoutAbstractsImplemented(members []*Symbol) []*Symbol {
	if !slices.ContainsFunc(members, isAbstractMethod) {
		return members
	}
	concrete := make(map[string]bool)
	for _, m := range members {
		if m.Kind == KindMethod && !m.IsAbstract {
			concrete[strings.ToLower(m.Name)] = true
		}
	}
	if len(concrete) == 0 {
		return members
	}
	return slices.DeleteFunc(members, func(m *Symbol) bool {
		return isAbstractMethod(m) && concrete[strings.ToLower(m.Name)]
	})
}

func isAbstractMethod(m *Symbol) bool {
	return m.Kind == KindMethod && m.IsAbstract
}

func (idx *Index) getClassMembersRecursive(fqn string, visited map[string]bool) []*Symbol {
	// A union, intersection or DNF type has the members of each class in it.
	if strings.ContainsAny(fqn, "|&(") {
		var members []*Symbol
		for _, part := range strings.FieldsFunc(fqn, func(r rune) bool { return strings.ContainsRune("|&()", r) }) {
			members = append(members, idx.getClassMembersRecursive(strings.TrimPrefix(strings.TrimPrefix(part, "?"), "\\"), visited)...)
		}
		return members
	}
	if visited[fqn] {
		return nil
	}
	visited[fqn] = true
	var own []*Symbol
	var rules *traitRules
	if sym, ok := idx.symbols[fqn]; ok {
		own, rules = sym.Children, sym.traitRules
		if sym.Kind == KindTrait {
			defer delete(visited, fqn)
		}
	}
	return idx.membersLocked(own, idx.traitMap[fqn], rules, idx.inheritanceMap[fqn], idx.implementsMap[fqn], visited)
}

func (idx *Index) Definitions(fqn string) []*Symbol {
	idx.ensureClass(fqn)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	current := idx.symbols[fqn]
	if current == nil {
		return nil
	}
	shadowed := idx.shadowed[fqn]
	defs := make([]*Symbol, 0, 1+len(shadowed))
	return append(append(defs, current), shadowed...)
}

func (idx *Index) DefinitionMembers(def *Symbol) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if idx.symbols[def.FQN] == def {
		return idx.getClassMembersRecursive(def.FQN, make(map[string]bool))
	}
	return idx.membersLocked(def.Children, def.Traits, def.traitRules, def.Extends, def.Implements, map[string]bool{def.FQN: true})
}

func (idx *Index) membersLocked(own []*Symbol, traits []string, rules *traitRules, parent string, interfaces []string, visited map[string]bool) []*Symbol {
	members := append([]*Symbol(nil), own...)
	// PHP's precedence: the class's own members, then its traits', then the
	// parent's. Callers take the first match, so the order matters.
	members = idx.appendTraitMembersLocked(members, traits, rules, visited)
	if parent != "" {
		members = append(members, idx.getClassMembersRecursive(parent, visited)...)
	}
	// Interfaces come last: their constants, and the methods nothing above
	// declares, which an abstract class may leave to its subclasses. A method
	// the class or a parent declares wins over the interface's signature.
	var declared map[string]bool
	for _, iface := range interfaces {
		for _, member := range idx.getClassMembersRecursive(iface, visited) {
			switch member.Kind {
			case KindConstant:
				members = append(members, member)
			case KindMethod:
				if declared == nil {
					declared = methodNames(members)
				}
				key := strings.ToLower(member.Name)
				if !declared[key] {
					declared[key] = true
					members = append(members, member)
				}
			}
		}
	}
	return members
}

// methodNames returns the lowercased names of the methods in members.
func methodNames(members []*Symbol) map[string]bool {
	names := make(map[string]bool, len(members))
	for _, m := range members {
		if m.Kind == KindMethod {
			names[strings.ToLower(m.Name)] = true
		}
	}
	return names
}

// HasAttributedSymbols reports whether anything indexed so far has attributes.
func (idx *Index) HasAttributedSymbols() bool {
	return idx.attributedSymbols.Load()
}

// OverridesInherited reports whether a class-like inherits a method of this name
// from a parent, interface or trait. known is false when one of those isn't
// indexed, so the answer can't be trusted.
func (idx *Index) OverridesInherited(classFQN, method string) (overrides, known bool) {
	return idx.overridesInherited(classFQN, method, KindMethod)
}

// OverridesInheritedConstant is OverridesInherited for a class constant or
// enum case, which PHP 8.6's #[\Override] may mark: whether an ancestor
// declares a constant of that name. Constant names are case-sensitive.
func (idx *Index) OverridesInheritedConstant(classFQN, name string) (overrides, known bool) {
	return idx.overridesInherited(classFQN, name, KindConstant)
}

func (idx *Index) overridesInherited(classFQN, method string, kind SymbolKind) (overrides, known bool) {
	idx.ensureClass(classFQN)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	known = true
	visited := map[string]bool{classFQN: true}
	var walk func(fqn string, viaTrait bool)
	walk = func(fqn string, viaTrait bool) {
		if visited[fqn] {
			return
		}
		visited[fqn] = true
		sym, ok := idx.symbols[fqn]
		if !ok {
			known = false
			return
		}
		for _, child := range sym.Children {
			// Of a trait's methods, only abstract ones are overridden rather than
			// just being the same method.
			sameName := strings.EqualFold(child.Name, method)
			if kind == KindConstant {
				sameName = child.Name == method
			}
			if child.Kind == kind && sameName && child.Visibility != "private" && (!viaTrait || child.IsAbstract) {
				overrides = true
			}
		}
		// A parent class's traits are part of that class, so their concrete
		// methods are overridden too; only the class's own traits (and the
		// traits they use) merely provide the same method.
		idx.walkAncestors(fqn, func(next string, isTrait bool) { walk(next, viaTrait && isTrait) })
	}
	idx.walkAncestors(classFQN, func(next string, isTrait bool) { walk(next, isTrait) })
	return overrides, known
}

func (idx *Index) walkAncestors(fqn string, visit func(next string, isTrait bool)) {
	if parent, ok := idx.inheritanceMap[fqn]; ok {
		visit(parent, false)
	}
	for _, iface := range idx.implementsMap[fqn] {
		visit(iface, false)
	}
	for _, tr := range idx.traitMap[fqn] {
		visit(tr, true)
	}
}

// IsSubtypeOf reports whether child is parent or extends, implements or uses
// it, directly or further up. Only child's ancestors are visited.
func (idx *Index) IsSubtypeOf(child, parent string) bool {
	idx.ensureClass(child)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.isSubtypeOfLocked(child, parent)
}

func (idx *Index) isSubtypeOfLocked(child, parent string) bool {
	if strings.EqualFold(child, parent) {
		return true
	}
	visited := map[string]bool{child: true}
	found := false
	var walk func(fqn string)
	walk = func(fqn string) {
		idx.walkAncestors(fqn, func(next string, _ bool) {
			if found || visited[next] {
				return
			}
			visited[next] = true
			if strings.EqualFold(next, parent) {
				found = true
				return
			}
			walk(next)
		})
	}
	walk(child)
	return found
}

func (idx *Index) GetInheritanceChain(classFQN string) []string {
	idx.ensureClass(classFQN)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var chain []string
	visited := make(map[string]bool)
	current := classFQN
	for {
		if visited[current] {
			break
		}
		visited[current] = true
		parent, ok := idx.inheritanceMap[current]
		if !ok || parent == "" {
			break
		}
		chain = append(chain, parent)
		current = parent
	}
	return chain
}

// GetDescendants returns all classes that directly or indirectly extend baseFQN.
func (idx *Index) GetDescendants(baseFQN string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var results []*Symbol
	for childFQN, parentFQN := range idx.inheritanceMap {
		if idx.isDescendantOf(childFQN, baseFQN, make(map[string]bool)) {
			if sym, ok := idx.symbols[childFQN]; ok {
				results = append(results, sym)
			}
		}
		_ = parentFQN
	}
	return results
}

func (idx *Index) isDescendantOf(fqn, baseFQN string, visited map[string]bool) bool {
	if visited[fqn] {
		return false
	}
	visited[fqn] = true
	parent, ok := idx.inheritanceMap[fqn]
	if !ok || parent == "" {
		return false
	}
	if parent == baseFQN {
		return true
	}
	return idx.isDescendantOf(parent, baseFQN, visited)
}

// AddVirtualMember adds a virtual member to an existing class symbol.
// Thread-safe — acquires the write lock.
func (idx *Index) AddVirtualMember(parentFQN string, member *Symbol) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.symbols[member.FQN] != nil {
		return // already exists
	}
	parent := idx.symbols[parentFQN]
	if parent == nil {
		return
	}
	member.ParentFQN = parentFQN
	parent.Children = append(parent.Children, member)
	idx.addSymbol(member.URI, member)
}

func (idx *Index) RefineProperty(fqn, fallbackType string, readonly bool) bool {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	existing := idx.symbols[fqn]
	if existing == nil {
		return false
	}
	refined := *existing
	if refined.Type == "" || refined.Type == "mixed" {
		refined.Type = fallbackType
	}
	refined.IsReadonly = refined.IsReadonly || readonly
	if refined.Type != existing.Type || refined.IsReadonly != existing.IsReadonly {
		idx.replaceSymbolLocked(existing, &refined)
	}
	return true
}

func (idx *Index) replaceSymbolLocked(old, replacement *Symbol) {
	idx.generation.Add(1)
	idx.symbols[old.FQN] = replacement
	idx.fileSymbols[old.URI] = withSymbolReplaced(idx.fileSymbols[old.URI], old, replacement)
	if parent := idx.symbols[old.ParentFQN]; parent != nil {
		parent.Children = withSymbolReplaced(parent.Children, old, replacement)
	}
}

func withSymbolReplaced(syms []*Symbol, old, replacement *Symbol) []*Symbol {
	for i, s := range syms {
		if s == old {
			out := append([]*Symbol(nil), syms...)
			out[i] = replacement
			return out
		}
	}
	return syms
}

func (idx *Index) GetImplementors(ifaceFQN string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var results []*Symbol
	seen := make(map[string]bool)
	for _, classFQN := range idx.reverseImplementsMap[ifaceFQN] {
		idx.collectImplementorLocked(classFQN, seen, &results)
	}
	return results
}

func (idx *Index) collectImplementorLocked(classFQN string, seen map[string]bool, results *[]*Symbol) {
	if seen[classFQN] {
		return
	}
	seen[classFQN] = true
	if sym, ok := idx.symbols[classFQN]; ok {
		*results = append(*results, sym)
	}
	for childFQN, parentFQN := range idx.inheritanceMap {
		if parentFQN == classFQN {
			idx.collectImplementorLocked(childFQN, seen, results)
		}
	}
}

func (idx *Index) GetImplementedInterfaces(classFQN string) []string {
	idx.ensureClass(classFQN)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.implementsMap[classFQN]
}

func (idx *Index) GetNamespaceMembers(ns string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var results []*Symbol
	for _, fqn := range idx.namespaceIndex[ns] {
		if sym, ok := idx.symbols[fqn]; ok {
			results = append(results, sym)
		}
	}
	return results
}

func symRange(line, col, nameLen int) protocol.Range {
	start := protocol.Position{Line: line, Character: col}
	end := protocol.Position{Line: line, Character: col + nameLen}
	return protocol.Range{Start: start, End: end}
}

func buildFQN(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "\\" + name
}

var phpBuiltinTypes = map[string]bool{
	"string": true, "int": true, "float": true, "bool": true, "array": true,
	"object": true, "callable": true, "iterable": true, "void": true, "never": true,
	"null": true, "false": true, "true": true, "mixed": true, "self": true,
	"static": true, "parent": true, "resource": true,
}

// IsPHPBuiltinType returns true if the name is a PHP primitive/built-in type.
func IsPHPBuiltinType(name string) bool {
	return phpBuiltinTypes[strings.ToLower(name)]
}

// IsBuiltin reports whether the symbol comes from the PHP builtin registry
// or embedded stubs. Built-in symbols are opaque to navigation: callers
// should not return a Location for them or scan the workspace for refs.
func IsBuiltin(sym *Symbol) bool {
	return sym != nil && sym.Source == SourceBuiltin
}

func resolveTypeName(name string, currentNs string, uses []parser.UseNode) string {
	if name == "" {
		return ""
	}
	// Strip nullable prefix for resolution, then re-add
	prefix := ""
	if strings.HasPrefix(name, "?") {
		prefix = "?"
		name = name[1:]
	}
	// Union, intersection and DNF types keep their shape: (App\A&App\B)|null.
	if strings.ContainsAny(name, "|&(") {
		var sb strings.Builder
		start := 0
		for i := 0; i <= len(name); i++ {
			if i < len(name) && !strings.ContainsRune("|&()", rune(name[i])) {
				continue
			}
			if i > start {
				sb.WriteString(resolveTypeName(name[start:i], currentNs, uses))
			}
			if i < len(name) {
				sb.WriteByte(name[i])
			}
			start = i + 1
		}
		return prefix + sb.String()
	}
	// Built-in types are never namespace-qualified
	if phpBuiltinTypes[strings.ToLower(name)] {
		return prefix + name
	}
	if strings.HasPrefix(name, "\\") {
		return prefix + strings.TrimPrefix(name, "\\")
	}
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range uses {
		if u.Alias == parts[0] {
			if len(parts) > 1 {
				return prefix + u.FullName + "\\" + parts[1]
			}
			return prefix + u.FullName
		}
	}
	if currentNs != "" {
		return prefix + currentNs + "\\" + name
	}
	return prefix + name
}

func splitTypeExpr(name string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(name); i++ {
		// A DNF type's parentheses, as in (A&B)|null, only group its parts.
		if name[i] == '|' || name[i] == '&' || name[i] == '(' || name[i] == ')' {
			if i > start {
				parts = append(parts, name[start:i])
			}
			start = i + 1
		}
	}
	if start < len(name) {
		parts = append(parts, name[start:])
	}
	return parts
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

type pendingRemovals struct {
	byName, byNamespace, byFold keyedRemovals
}

type keyedRemovals map[string][]string

func (r *keyedRemovals) add(key, fqn string) {
	if *r == nil {
		*r = make(keyedRemovals)
	}
	(*r)[key] = append((*r)[key], fqn)
}

func (r keyedRemovals) take(key, fqn string) bool {
	fqns := r[key]
	i := slices.Index(fqns, fqn)
	if i < 0 {
		return false
	}
	if fqns = slices.Delete(fqns, i, i+1); len(fqns) > 0 {
		r[key] = fqns
	} else {
		delete(r, key)
	}
	return true
}

func (r keyedRemovals) applyTo(index map[string][]string) {
	for key, fqns := range r {
		if kept := withoutAll(index[key], fqns); len(kept) > 0 {
			index[key] = kept
		} else {
			delete(index, key)
		}
	}
}

func withoutAll(slice, drop []string) []string {
	kept := slice[:0]
	if len(drop) == 1 {
		for _, s := range slice {
			if s != drop[0] {
				kept = append(kept, s)
			}
		}
	} else {
		dropped := make(map[string]struct{}, len(drop))
		for _, s := range drop {
			dropped[s] = struct{}{}
		}
		for _, s := range slice {
			if _, ok := dropped[s]; !ok {
				kept = append(kept, s)
			}
		}
	}
	clear(slice[len(kept):])
	return kept
}

func removeFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// GetAllFileURIs returns all file URIs that have been indexed.
func (idx *Index) GetAllFileURIs() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	uris := make([]string, 0, len(idx.fileSymbols))
	for uri := range idx.fileSymbols {
		if idx.generatedURIs[uri] {
			continue // not a file on disk
		}
		uris = append(uris, uri)
	}
	return uris
}

// AddTemplateFile records a template (a Blade view) in the workspace. Only its
// URI is kept.
func (idx *Index) AddTemplateFile(uri string) {
	idx.mu.Lock()
	if idx.templateFiles == nil {
		idx.templateFiles = make(map[string]struct{})
	}
	idx.templateFiles[uri] = struct{}{}
	idx.mu.Unlock()
}

// TemplateFileURIs returns the URIs recorded with AddTemplateFile, as a new
// slice.
func (idx *Index) TemplateFileURIs() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	uris := make([]string, 0, len(idx.templateFiles))
	for uri := range idx.templateFiles {
		uris = append(uris, uri)
	}
	return uris
}

// GetFileSource returns the raw PHP source text stored for the given URI,
// or an empty string if the URI has not been indexed or was removed.
// This allows the analyzer to avoid re-reading files from disk.
func (idx *Index) GetFileSource(uri string) string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.fileSource[uri].text
}

// Ready reports whether at least one full workspace + composer indexing pass
// has completed. Diagnostic rules that depend on cross-file symbol presence
// (unknown-class, unknown-function, unknown-member) should consult this and
// suppress findings during the warm-up window.
func (idx *Index) Ready() bool {
	return idx.ready.Load()
}

// MarkReady flips the index into the ready state. Called once by the LSP
// server after the async workspace + composer indexers finish. Safe to call
// multiple times.
func (idx *Index) MarkReady() {
	idx.ready.Store(true)
	idx.loader.Store(nil)
}

// TraitsOf returns a copy of the trait FQNs used by the class identified by
// fqn. Returns nil when the class uses no traits or is not indexed. The
// returned slice is safe to use after the lock is released.
func (idx *Index) TraitsOf(fqn string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return append([]string(nil), idx.traitMap[fqn]...)
}

// PickBestStandalone selects the most appropriate symbol when multiple symbols
// share the same short name and the word appears in standalone context (not
// after -> or ::). Prefers functions over classes over constants over
// methods/properties. Among equally-ranked kinds, prefers an exact case match.
func PickBestStandalone(syms []*Symbol, word string) *Symbol {
	var best *Symbol
	bestRank := 999
	bestExact := false

	for _, s := range syms {
		// Properties are never standalone in PHP — they require -> or :: access.
		if s.Kind == KindProperty {
			continue
		}
		r := standaloneRank(s)
		exact := s.Name == word
		if r < bestRank || (r == bestRank && exact && !bestExact) {
			best = s
			bestRank = r
			bestExact = exact
		}
	}
	return best
}

func standaloneRank(s *Symbol) int {
	switch s.Kind {
	case KindFunction:
		return 0
	case KindClass, KindInterface, KindEnum, KindTrait:
		return 1
	case KindConstant, KindEnumCase:
		return 2
	default: // KindMethod, KindProperty
		return 3
	}
}

// setParentLocked records that fqn extends parent, in both directions.
func (idx *Index) setParentLocked(fqn, parent string) {
	if old, ok := idx.inheritanceMap[fqn]; ok && old != parent {
		key := strings.ToLower(old)
		idx.childrenMap[key] = removeFromSlice(idx.childrenMap[key], fqn)
	}
	idx.inheritanceMap[fqn] = parent
	key := strings.ToLower(parent)
	idx.childrenMap[key] = appendUnique(idx.childrenMap[key], fqn)
}

// Implementations returns every class-like below fqn: the classes that extend
// it, the interfaces that extend it, the classes that implement any of those,
// and so on down, nearest first. Like Subclasses, it follows reverse maps and
// visits only what is below fqn.
func (idx *Index) Implementations(fqn string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []string
	seen := map[string]bool{strings.ToLower(fqn): true}
	queue := []string{fqn}
	visit := func(names []string) {
		for _, name := range names {
			key := strings.ToLower(name)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, name)
			queue = append(queue, name)
		}
	}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		idx.directSubtypesLocked(next, visit)
	}
	return out
}

func (idx *Index) DirectSubtypes(fqn string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []string
	seen := map[string]bool{strings.ToLower(fqn): true}
	idx.directSubtypesLocked(fqn, func(names []string) {
		for _, name := range names {
			if key := strings.ToLower(name); !seen[key] {
				seen[key] = true
				out = append(out, name)
			}
		}
	})
	return out
}

func (idx *Index) directSubtypesLocked(fqn string, visit func([]string)) {
	visit(idx.childrenMap[strings.ToLower(fqn)])
	visit(idx.reverseImplementsMap[fqn])
	visit(idx.subInterfaceMap[fqn])
}

func (idx *Index) DirectSupertypes(fqn string) []string {
	idx.ensureGenerated(fqn)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []string
	idx.walkAncestors(fqn, func(next string, _ bool) { out = append(out, next) })
	return out
}

// Subclasses returns every class that extends fqn, directly or further down,
// nearest first. It follows the parent → children map, so it visits only the
// classes below fqn rather than every inheritance edge. Parent names match
// regardless of case, as in PHP.
func (idx *Index) Subclasses(fqn string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []string
	seen := map[string]bool{strings.ToLower(fqn): true}
	queue := []string{fqn}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, child := range idx.childrenMap[strings.ToLower(next)] {
			key := strings.ToLower(child)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, child)
			queue = append(queue, child)
		}
	}
	return out
}
