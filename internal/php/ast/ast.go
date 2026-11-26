package ast

import "github.com/Tusk-PHP/lsp/internal/php/token"

//go:generate go run ./gen

type Span struct {
	Start uint32
	End   uint32
}

func (s Span) Bounds() Span { return s }

func (s Span) Text(src string) string { return src[s.Start:s.End] }

type Node interface {
	Bounds() Span
}

type Expr interface {
	Node
	exprNode()
	Parenthesized() bool
	markParenthesized()
}

type Stmt interface {
	Node
	stmtNode()
}

type Member interface {
	Node
	memberNode()
}

type Type interface {
	Node
	typeNode()
}

type expr struct {
	parenthesized bool
}

func (expr) exprNode() {}

func (e expr) Parenthesized() bool { return e.parenthesized }

func (e *expr) markParenthesized() { e.parenthesized = true }

func Parenthesize(e Expr) {
	e.markParenthesized()
}

type stmt struct{}

func (stmt) stmtNode() {}

type member struct{}

func (member) memberNode() {}

type typ struct{}

func (typ) typeNode() {}

type File struct {
	Span
	Shebang  Span
	Stmts    []Stmt
	Errors   []Error
	Warnings []Error
	Version  token.Version
}

func (f *File) Clean() bool {
	return len(f.Errors) == 0 && len(f.Warnings) == 0
}

type Error struct {
	Span
	Message string
}

type Name struct {
	Span
	Kind token.Kind
}

type Identifier struct {
	Span
}

type Modifier struct {
	Span
	Kind token.Kind
}

type Attribute struct {
	Span
	Name *Name
	Args *ArgList
}

type AttributeGroup struct {
	Span
	Attrs []*Attribute
}

type NamedType struct {
	typ
	Span
	Name *Name
}

type NullableType struct {
	typ
	Span
	Inner Type
}

type UnionType struct {
	typ
	Span
	Types []Type
}

type IntersectionType struct {
	typ
	Span
	Types []Type
}

type Param struct {
	Span
	Attrs     []*AttributeGroup
	Doc       Span
	Modifiers []*Modifier
	Type      Type
	ByRef     bool
	Variadic  bool
	Var       *Variable
	Default   Expr
	Hooks     []*PropertyHook
}

type Arg struct {
	Span
	Name        *Identifier
	Unpack      bool
	Placeholder bool
	Value       Expr
}

type ArgList struct {
	Span
	Args            []*Arg
	RestPlaceholder bool
}

type Variable struct {
	expr
	Span
}

type VariableVariable struct {
	expr
	Span
	Name Expr
}

type Literal struct {
	Span
	Kind token.Kind
	expr
}

type InterpolatedString struct {
	Span
	Kind token.Kind
	expr
	Parts []Expr
}

type StringPart struct {
	expr
	Span
}

type MagicConst struct {
	Span
	Kind token.Kind
	expr
}

type ConstFetch struct {
	expr
	Span
	Name *Name
}

type ArrayItem struct {
	Span
	Key    Expr
	Value  Expr
	ByRef  bool
	Unpack bool
}

type Array struct {
	Span
	Short bool
	expr
	Items []*ArrayItem
}

type List struct {
	Span
	Short bool
	expr
	Items []*ArrayItem
}

type Binary struct {
	Span
	Op token.Kind
	expr
	Left  Expr
	Right Expr
}

type Unary struct {
	Span
	Op token.Kind
	expr
	Operand Expr
}

type Postfix struct {
	Span
	Op token.Kind
	expr
	Operand Expr
}

type Assign struct {
	Span
	Op    token.Kind
	ByRef bool
	expr
	Target Expr
	Value  Expr
}

type Ternary struct {
	expr
	Span
	Cond Expr
	Then Expr
	Else Expr
}

type Cast struct {
	Span
	Kind token.Kind
	expr
	Operand Expr
}

type InstanceOf struct {
	expr
	Span
	Operand Expr
	Class   Expr
}

type Isset struct {
	expr
	Span
	Vars []Expr
}

type Empty struct {
	expr
	Span
	Operand Expr
}

type Include struct {
	Span
	Kind token.Kind
	expr
	Operand Expr
}

type Eval struct {
	expr
	Span
	Operand Expr
}

type Exit struct {
	expr
	Span
	Args *ArgList
}

type Print struct {
	expr
	Span
	Operand Expr
}

type Clone struct {
	expr
	Span
	Operand Expr
	Args    *ArgList
}

type Throw struct {
	expr
	Span
	Operand Expr
}

type Yield struct {
	expr
	Span
	Key   Expr
	Value Expr
}

type YieldFrom struct {
	expr
	Span
	Operand Expr
}

type Call struct {
	expr
	Span
	Callee Expr
	Args   *ArgList
}

type New struct {
	expr
	Span
	Class     Expr
	Anonymous *ClassDecl
	Args      *ArgList
}

type MethodCall struct {
	Span
	Object   Expr
	Name     Expr
	Nullsafe bool
	expr
	Args *ArgList
}

type StaticCall struct {
	expr
	Span
	Class Expr
	Name  Expr
	Args  *ArgList
}

type PropertyFetch struct {
	Span
	Object   Expr
	Name     Expr
	Nullsafe bool
	expr
}

type StaticPropertyFetch struct {
	expr
	Span
	Class Expr
	Name  Expr
}

type ClassConstFetch struct {
	expr
	Span
	Class Expr
	Name  Expr
}

type ArrayDimFetch struct {
	expr
	Span
	Array Expr
	Dim   Expr
}

type IdentifierExpr struct {
	expr
	Span
}

type ClosureUse struct {
	Span
	ByRef bool
	Var   *Variable
}

type Closure struct {
	Span
	Attrs  []*AttributeGroup
	Static bool
	ByRef  bool
	expr
	Params     []*Param
	Uses       []*ClosureUse
	ReturnType Type
	Body       *Block
}

type ArrowFunction struct {
	Span
	Attrs  []*AttributeGroup
	Static bool
	ByRef  bool
	expr
	Params     []*Param
	ReturnType Type
	Body       Expr
}

type MatchArm struct {
	Span
	Conds []Expr
	Body  Expr
}

type Match struct {
	expr
	Span
	Subject Expr
	Arms    []*MatchArm
}

type BadExpr struct {
	expr
	Span
}

type InlineHTML struct {
	stmt
	Span
}

type Echo struct {
	stmt
	Span
	Exprs []Expr
}

type ExprStmt struct {
	stmt
	Span
	Expr Expr
}

type Block struct {
	stmt
	Span
	Stmts []Stmt
}

type ElseIf struct {
	Span
	Cond Expr
	Body Stmt
}

type If struct {
	stmt
	Span
	Cond    Expr
	Then    Stmt
	ElseIfs []*ElseIf
	Else    Stmt
}

type While struct {
	stmt
	Span
	Cond Expr
	Body Stmt
}

type DoWhile struct {
	stmt
	Span
	Body Stmt
	Cond Expr
}

type For struct {
	stmt
	Span
	Init []Expr
	Cond []Expr
	Loop []Expr
	Body Stmt
}

type Foreach struct {
	stmt
	Span
	Subject Expr
	Key     Expr
	Value   Expr
	ByRef   bool
	Body    Stmt
}

type Case struct {
	Span
	Cond  Expr
	Stmts []Stmt
}

type Switch struct {
	stmt
	Span
	Subject Expr
	Cases   []*Case
}

type Break struct {
	stmt
	Span
	Levels Expr
}

type Continue struct {
	stmt
	Span
	Levels Expr
}

type Return struct {
	stmt
	Span
	Value Expr
}

type Global struct {
	stmt
	Span
	Vars []Expr
}

type StaticVar struct {
	Span
	Var     *Variable
	Default Expr
}

type Static struct {
	stmt
	Span
	Vars []*StaticVar
}

type Unset struct {
	stmt
	Span
	Vars []Expr
}

type Catch struct {
	Span
	Types []*Name
	Var   *Variable
	Body  *Block
}

type Try struct {
	stmt
	Span
	Body    *Block
	Catches []*Catch
	Finally *Block
}

type Goto struct {
	stmt
	Span
	Label *Identifier
}

type Label struct {
	stmt
	Span
	Name *Identifier
}

type DeclareDirective struct {
	Span
	Name  *Identifier
	Value Expr
}

type Declare struct {
	stmt
	Span
	Directives []*DeclareDirective
	Body       Stmt
}

type Nop struct {
	stmt
	Span
}

type HaltCompiler struct {
	stmt
	Span
}

type Namespace struct {
	stmt
	Span
	Name   *Name
	Braced bool
	Stmts  []Stmt
}

type UseItem struct {
	Span
	Kind  token.Kind
	Name  *Name
	Alias *Identifier
}

type Use struct {
	stmt
	Span
	Kind   token.Kind
	Prefix *Name
	Items  []*UseItem
}

type ConstItem struct {
	Span
	Name  *Identifier
	Value Expr
}

type Const struct {
	stmt
	Span
	Attrs []*AttributeGroup
	Doc   Span
	Items []*ConstItem
}

type FunctionDecl struct {
	stmt
	Span
	Attrs      []*AttributeGroup
	Doc        Span
	ByRef      bool
	Name       *Identifier
	Params     []*Param
	ReturnType Type
	Body       *Block
}

type ClassDecl struct {
	stmt
	Span
	Attrs      []*AttributeGroup
	Doc        Span
	Modifiers  []*Modifier
	Name       *Identifier
	Args       *ArgList
	Extends    *Name
	Implements []*Name
	Members    []Member
}

type InterfaceDecl struct {
	stmt
	Span
	Attrs   []*AttributeGroup
	Doc     Span
	Name    *Identifier
	Extends []*Name
	Members []Member
}

type TraitDecl struct {
	stmt
	Span
	Attrs   []*AttributeGroup
	Doc     Span
	Name    *Identifier
	Members []Member
}

type EnumDecl struct {
	stmt
	Span
	Attrs       []*AttributeGroup
	Doc         Span
	Name        *Identifier
	BackingType Type
	Implements  []*Name
	Members     []Member
}

type ClassConst struct {
	member
	Span
	Attrs     []*AttributeGroup
	Doc       Span
	Modifiers []*Modifier
	Type      Type
	Items     []*ConstItem
}

type PropertyItem struct {
	Span
	Var     *Variable
	Default Expr
}

type PropertyHook struct {
	Span
	Attrs     []*AttributeGroup
	Modifiers []*Modifier
	ByRef     bool
	Name      *Identifier
	Params    []*Param
	Body      Node
}

type Property struct {
	member
	Span
	Attrs     []*AttributeGroup
	Doc       Span
	Modifiers []*Modifier
	Type      Type
	Items     []*PropertyItem
	Hooks     []*PropertyHook
}

type Method struct {
	member
	Span
	Attrs      []*AttributeGroup
	Doc        Span
	Modifiers  []*Modifier
	ByRef      bool
	Name       *Identifier
	Params     []*Param
	ReturnType Type
	Body       *Block
}

type TraitAdaptation struct {
	Span
	Trait     *Name
	Method    *Identifier
	InsteadOf []*Name
	Modifier  *Modifier
	Alias     *Identifier
}

type TraitUse struct {
	member
	Span
	Doc         Span
	Traits      []*Name
	Adaptations []*TraitAdaptation
}

type EnumCase struct {
	member
	Span
	Attrs []*AttributeGroup
	Doc   Span
	Name  *Identifier
	Value Expr
}

type BadMember struct {
	member
	Span
}

type BadStmt struct {
	stmt
	Span
}
