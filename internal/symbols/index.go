package symbols

import (
	"net/url"
	"path/filepath"
	"runtime"
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
	KindVariable
	KindEnumCase
	KindNamespace
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
	Extends       string
	BackedType    string
	Value         string
	IsVirtual     bool
	Templates     []TemplateParam
	Supertypes    []Supertype
	TypeAliases   map[string]TypeAlias
	Hooks         []parser.PropertyHook // PHP 8.4 property hooks (get/set accessors)
	Attributes    []Attribute
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
	nameIndex            map[string][]string
	foldIndex            map[string][]string // lowercase FQN → FQNs of class-likes and functions, whose names PHP matches case-insensitively
	fileSymbols          map[string][]*Symbol
	namespaceIndex       map[string][]string
	inheritanceMap       map[string]string
	childrenMap          map[string][]string // lowercase parent FQN → direct subclass FQNs, the reverse of inheritanceMap
	implementsMap        map[string][]string // class → interfaces it implements
	reverseImplementsMap map[string][]string // interface → classes that implement it
	subInterfaceMap      map[string][]string // interface → interfaces that extend it
	traitMap             map[string][]string
	sortedNames          []string          // sorted lowercase name keys for binary search
	sortedNamesDirty     bool              // rebuild flag
	sortedFQNs           []string          // sorted FQN keys for binary search (SearchByFQNPrefix)
	sortedFQNsDirty      bool              // rebuild flag for sortedFQNs
	fileSource           map[string]string // URI → raw source text stored during IndexFileWithSource
	ready                atomic.Bool       // true once project + vendor indexing has completed at least once
	generator            atomic.Pointer[classGeneratorHolder]
	generatedURIs        map[string]bool     // URIs of generated classes, which are not files on disk
	generatedFrom        map[string][]string // lowercase source FQN → URIs of the classes generated from it
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
		fileSource:           make(map[string]string),
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
	// Store the raw source text for this URI, mirroring IndexFileWithSource.
	idx.fileSource[uri] = source
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
			var resolvedImpls []string
			for _, impl := range c.Implements {
				resolvedImpls = append(resolvedImpls, resolve(impl))
			}
			sym := &Symbol{Name: c.Name, FQN: fqn, Kind: KindClass, URI: uri, DocComment: qualifyDocTypes(c.DocComment, file.Uses),
				IsAbstract: c.IsAbstract, IsFinal: c.IsFinal, IsReadonly: c.IsReadonly,
				Implements: resolvedImpls,
				Range:      symRange(c.StartLine, c.StartCol, len(c.Name))}
			if c.Extends != "" {
				sym.Extends = resolve(c.Extends)
			}
			idx.addSymbolWithSource(uri, sym, SourceProject)
			if c.Extends != "" {
				idx.setParentLocked(fqn, sym.Extends)
			}
			for _, impl := range resolvedImpls {
				idx.implementsMap[fqn] = append(idx.implementsMap[fqn], impl)
				idx.reverseImplementsMap[impl] = appendUnique(idx.reverseImplementsMap[impl], fqn)
			}
			for _, tr := range c.Traits {
				idx.traitMap[fqn] = append(idx.traitMap[fqn], resolve(tr))
			}
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

func (idx *Index) IndexFileWithSource(uri string, source string, src SymbolSource) {
	file := parser.ParseFile(source)
	if file == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeFileSymbols(uri)
	// Store the raw source text for this URI (replaces any previous version)
	idx.fileSource[uri] = source
	idx.indexParsedLocked(uri, file, src)
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
		var resolvedImpls []string
		for _, impl := range c.Implements {
			resolvedImpls = append(resolvedImpls, resolve(impl))
		}
		sym := &Symbol{Name: c.Name, FQN: fqn, Kind: KindClass, URI: uri, DocComment: qualifyDocTypes(c.DocComment, file.Uses),
			IsAbstract: c.IsAbstract, IsFinal: c.IsFinal, IsReadonly: c.IsReadonly,
			Implements: resolvedImpls, Attributes: indexAttributes(c.Attributes, resolve),
			Range: symRange(c.StartLine, c.StartCol, len(c.Name))}
		previous := idx.symbols[fqn]
		if c.Extends != "" {
			sym.Extends = resolve(c.Extends)
		}
		idx.addSymbolWithSource(uri, sym, src)
		if c.Extends != "" {
			idx.setParentLocked(fqn, sym.Extends)
		}
		for _, impl := range resolvedImpls {
			idx.implementsMap[fqn] = append(idx.implementsMap[fqn], impl)
			idx.reverseImplementsMap[impl] = appendUnique(idx.reverseImplementsMap[impl], fqn)
		}
		for _, tr := range c.Traits {
			idx.traitMap[fqn] = append(idx.traitMap[fqn], resolve(tr))
		}
		idx.indexProperties(uri, sym, fqn, c.Properties, src, resolve, file.Uses)
		idx.indexMethods(uri, sym, fqn, c.Methods, src, resolve, file.Uses)
		idx.indexConstants(uri, sym, fqn, c.Constants, src, resolve)
		idx.indexVirtualMembers(uri, sym, src, resolve)
		sym.Supertypes = append(sym.Supertypes, traitUseSupertypes(c.TraitUseDoc, file.Uses, resolve)...)
		if src == SourceBuiltin {
			mergeEarlierLayer(sym, previous)
		}
	}

	for _, iface := range file.Interfaces {
		fqn := buildFQN(ns, iface.Name)
		var resolvedParents []string
		for _, parent := range iface.Extends {
			resolvedParents = append(resolvedParents, resolve(parent))
		}
		sym := &Symbol{Name: iface.Name, FQN: fqn, Kind: KindInterface, URI: uri, DocComment: qualifyDocTypes(iface.DocComment, file.Uses),
			Implements: resolvedParents, Attributes: indexAttributes(iface.Attributes, resolve),
			Range: symRange(iface.StartLine, iface.StartCol, len(iface.Name))}
		idx.addSymbolWithSource(uri, sym, src)
		for _, parent := range resolvedParents {
			idx.implementsMap[fqn] = append(idx.implementsMap[fqn], parent)
			idx.subInterfaceMap[parent] = appendUnique(idx.subInterfaceMap[parent], fqn)
		}
		idx.indexProperties(uri, sym, fqn, iface.Properties, src, resolve, file.Uses)
		idx.indexMethods(uri, sym, fqn, iface.Methods, src, resolve, file.Uses)
		idx.indexConstants(uri, sym, fqn, iface.Constants, src, resolve)
		idx.indexVirtualMembers(uri, sym, src, resolve)
	}

	for _, tr := range file.Traits {
		fqn := buildFQN(ns, tr.Name)
		sym := &Symbol{Name: tr.Name, FQN: fqn, Kind: KindTrait, URI: uri, DocComment: qualifyDocTypes(tr.DocComment, file.Uses),
			Range: symRange(tr.StartLine, tr.StartCol, len(tr.Name))}
		idx.addSymbolWithSource(uri, sym, src)
		for _, usedTrait := range tr.Traits {
			idx.traitMap[fqn] = append(idx.traitMap[fqn], resolve(usedTrait))
		}
		idx.indexProperties(uri, sym, fqn, tr.Properties, src, resolve, file.Uses)
		idx.indexMethods(uri, sym, fqn, tr.Methods, src, resolve, file.Uses)
		idx.indexConstants(uri, sym, fqn, tr.Constants, src, resolve)
		idx.indexVirtualMembers(uri, sym, src, resolve)
		sym.Supertypes = append(sym.Supertypes, traitUseSupertypes(tr.TraitUseDoc, file.Uses, resolve)...)
	}

	for _, en := range file.Enums {
		fqn := buildFQN(ns, en.Name)
		var resolvedEnumImpls []string
		for _, impl := range en.Implements {
			resolvedEnumImpls = append(resolvedEnumImpls, resolve(impl))
		}
		sym := &Symbol{Name: en.Name, FQN: fqn, Kind: KindEnum, URI: uri, DocComment: qualifyDocTypes(en.DocComment, file.Uses),
			BackedType: en.BackedType, Implements: resolvedEnumImpls, Attributes: indexAttributes(en.Attributes, resolve),
			Range: symRange(en.StartLine, en.StartCol, len(en.Name))}
		idx.addSymbolWithSource(uri, sym, src)
		for _, impl := range resolvedEnumImpls {
			idx.implementsMap[fqn] = append(idx.implementsMap[fqn], impl)
			idx.reverseImplementsMap[impl] = appendUnique(idx.reverseImplementsMap[impl], fqn)
		}
		for _, ec := range en.Cases {
			cs := &Symbol{Name: ec.Name, FQN: fqn + "::" + ec.Name, Kind: KindEnumCase, URI: uri, ParentFQN: fqn, Value: ec.Value,
				DocComment: ec.DocComment, Attributes: indexAttributes(ec.Attributes, resolve),
				Range: symRange(ec.StartLine, ec.StartCol, len(ec.Name))}
			sym.Children = append(sym.Children, cs)
			idx.addSymbolWithSource(uri, cs, src)
		}
		for _, tr := range en.Traits {
			idx.traitMap[fqn] = append(idx.traitMap[fqn], resolve(tr))
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
	if sym.Source != SourceGenerated && isTopLevelSymbol(sym.Kind) {
		// A real declaration replaces a class generated under the same name.
		idx.dropGeneratedLocked(sym.FQN)
	}
	idx.symbols[sym.FQN] = sym
	if len(sym.Attributes) > 0 {
		idx.attributedSymbols.Store(true)
	}
	idx.fileSymbols[uri] = append(idx.fileSymbols[uri], sym)
	if sym.Kind == KindClass && sym.Name == parser.AnonymousClassName {
		// Found by its FQN only: no one can write its name.
		return
	}
	idx.nameIndex[sym.Name] = appendUnique(idx.nameIndex[sym.Name], sym.FQN)
	idx.sortedNamesDirty = true
	idx.sortedFQNsDirty = true
	// Index top-level symbols by their namespace
	if isTopLevelSymbol(sym.Kind) {
		idx.namespaceIndex[namespaceForFQN(sym.FQN)] = appendUnique(idx.namespaceIndex[namespaceForFQN(sym.FQN)], sym.FQN)
		idx.addFoldLocked(sym.FQN)
	}
}

func (idx *Index) addFoldLocked(fqn string) {
	key := strings.ToLower(fqn)
	idx.foldIndex[key] = appendUnique(idx.foldIndex[key], fqn)
}

// LookupFold finds a class-like or function the way PHP does, ignoring case:
// `new stdclass`, `PclZip` declared as `pclzip`, `floatVal()`. An exact match
// wins. match, when not nil, must accept the symbol (for example to want a
// class and not a function of the same name).
func (idx *Index) LookupFold(fqn string, match func(*Symbol) bool) *Symbol {
	idx.ensureGenerated(fqn)
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
	for _, sym := range idx.fileSymbols[uri] {
		if narrowsTypes(sym) {
			key := strings.ToLower(sym.Name)
			if idx.narrowingNames[key] <= 1 {
				delete(idx.narrowingNames, key)
			} else {
				idx.narrowingNames[key]--
			}
		}
		if isTopLevelSymbol(sym.Kind) {
			// Classes generated from this one are rebuilt on their next lookup.
			idx.dropGeneratedFromLocked(sym.FQN)
		}
		delete(idx.symbols, sym.FQN)
		if fqns, ok := idx.nameIndex[sym.Name]; ok {
			idx.nameIndex[sym.Name] = removeFromSlice(fqns, sym.FQN)
		}
		idx.sortedNamesDirty = true
		idx.sortedFQNsDirty = true
		// Clean up namespace index
		if isTopLevelSymbol(sym.Kind) {
			ns := namespaceForFQN(sym.FQN)
			if fqns, ok := idx.namespaceIndex[ns]; ok {
				idx.namespaceIndex[ns] = removeFromSlice(fqns, sym.FQN)
			}
			key := strings.ToLower(sym.FQN)
			if fqns := removeFromSlice(idx.foldIndex[key], sym.FQN); len(fqns) > 0 {
				idx.foldIndex[key] = fqns
			} else {
				delete(idx.foldIndex, key)
			}
		}
		// Clean up reverse implements map
		if ifaces, ok := idx.implementsMap[sym.FQN]; ok {
			for _, iface := range ifaces {
				idx.reverseImplementsMap[iface] = removeFromSlice(idx.reverseImplementsMap[iface], sym.FQN)
				if sym.Kind == KindInterface {
					idx.subInterfaceMap[iface] = removeFromSlice(idx.subInterfaceMap[iface], sym.FQN)
				}
			}
		}
		delete(idx.implementsMap, sym.FQN)
		if parent, ok := idx.inheritanceMap[sym.FQN]; ok {
			key := strings.ToLower(parent)
			if children := removeFromSlice(idx.childrenMap[key], sym.FQN); len(children) > 0 {
				idx.childrenMap[key] = children
			} else {
				delete(idx.childrenMap, key)
			}
		}
		delete(idx.inheritanceMap, sym.FQN)
		delete(idx.traitMap, sym.FQN)
	}
	delete(idx.fileSymbols, uri)
	delete(idx.fileSource, uri)
	delete(idx.generatedURIs, uri)
}

func isTopLevelSymbol(kind SymbolKind) bool {
	return kind != KindMethod && kind != KindProperty && kind != KindConstant && kind != KindEnumCase
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
	if len(sym.Implements) == 0 {
		sym.Implements = previous.Implements
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
	if sym == nil && idx.ensureGenerated(fqn) {
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

	// Ensure the sorted names slice is up-to-date under a write lock if needed,
	// then perform the binary search under a read lock.
	idx.mu.RLock()
	needRebuild := idx.sortedNamesDirty || len(idx.sortedNames) == 0
	idx.mu.RUnlock()

	if needRebuild {
		idx.mu.Lock()
		// Re-check under write lock to avoid duplicate work when two goroutines
		// both observed dirty and one completes the rebuild before the other.
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
		idx.mu.Unlock()
	}

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
		if sym.Kind == KindMethod || sym.Kind == KindProperty || sym.Kind == KindConstant || sym.Kind == KindEnumCase {
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
	idx.ensureGenerated(classFQN)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.getClassMembersRecursive(classFQN, make(map[string]bool))
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
	var members []*Symbol
	if sym, ok := idx.symbols[fqn]; ok {
		members = append(members, sym.Children...)
	}
	// PHP's precedence: the class's own members, then its traits', then the
	// parent's. Callers take the first match, so the order matters.
	if traits, ok := idx.traitMap[fqn]; ok {
		for _, tr := range traits {
			members = append(members, idx.getClassMembersRecursive(tr, visited)...)
		}
	}
	if parent, ok := idx.inheritanceMap[fqn]; ok {
		members = append(members, idx.getClassMembersRecursive(parent, visited)...)
	}
	// Interfaces are read for their constants only: the methods they declare are
	// the class's own to implement.
	if interfaces, ok := idx.implementsMap[fqn]; ok {
		for _, iface := range interfaces {
			for _, member := range idx.getClassMembersRecursive(iface, visited) {
				if member.Kind == KindConstant {
					members = append(members, member)
				}
			}
		}
	}
	return members
}

// HasAttributedSymbols reports whether anything indexed so far has attributes.
func (idx *Index) HasAttributedSymbols() bool {
	return idx.attributedSymbols.Load()
}

// OverridesInherited reports whether a class-like inherits a method of this name
// from a parent, interface or trait. known is false when one of those isn't
// indexed, so the answer can't be trusted.
func (idx *Index) OverridesInherited(classFQN, method string) (overrides, known bool) {
	idx.ensureGenerated(classFQN)
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
			if child.Kind == KindMethod && strings.EqualFold(child.Name, method) && child.Visibility != "private" && (!viaTrait || child.IsAbstract) {
				overrides = true
			}
		}
		idx.walkAncestors(fqn, func(next string, isTrait bool) { walk(next, viaTrait || isTrait) })
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
	if strings.EqualFold(child, parent) {
		return true
	}
	idx.ensureGenerated(child)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
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
	idx.ensureGenerated(classFQN)
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
	idx.ensureGenerated(classFQN)
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

// GetFileSource returns the raw PHP source text stored for the given URI,
// or an empty string if the URI has not been indexed or was removed.
// This allows the analyzer to avoid re-reading files from disk.
func (idx *Index) GetFileSource(uri string) string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.fileSource[uri]
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
}

// TraitsOf returns a copy of the trait FQNs used by the class identified by
// fqn. Returns nil when the class uses no traits or is not indexed. The
// returned slice is safe to use after the lock is released.
func (idx *Index) TraitsOf(fqn string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return append([]string(nil), idx.traitMap[fqn]...)
}

func URIToPath(uri string) string {
	if u, err := url.Parse(uri); err == nil && u.Scheme == "file" {
		// url.Parse decodes percent-encoding in the path (e.g. %3A → :)
		p := u.Path
		// On Windows, file URIs look like file:///C:/... → Path="/C:/..."
		// Strip the leading slash before the drive letter.
		if runtime.GOOS == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
			p = p[1:]
		}
		return filepath.Clean(p)
	}
	// Not a URI — treat as a plain path.
	return filepath.Clean(uri)
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
		visit(idx.childrenMap[strings.ToLower(next)])
		visit(idx.reverseImplementsMap[next])
		visit(idx.subInterfaceMap[next])
	}
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
