package token

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Version uint8

const (
	PHP74 Version = iota
	PHP80
	PHP81
	PHP82
	PHP83
	PHP84
	PHP85
	PHP86
	Latest = PHP86
)

func ParseVersion(s string) (Version, error) {
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 2 {
		return 0, fmt.Errorf("PHP version %q: want major.minor", s)
	}
	major, errMajor := strconv.ParseUint(parts[0], 10, 8)
	minor, errMinor := strconv.ParseUint(parts[1], 10, 8)
	if err := errors.Join(errMajor, errMinor); err != nil {
		return 0, fmt.Errorf("PHP version %q: want major.minor: %w", s, err)
	}
	switch {
	case major < 8:
		return PHP74, nil
	case major > 8 || minor > uint64(Latest-PHP80):
		return Latest, nil
	}
	return PHP80 + Version(minor), nil
}

func (v Version) String() string {
	if v == PHP74 {
		return "7.4"
	}
	return "8." + strconv.Itoa(int(v-PHP80))
}

type Kind uint16

const (
	Invalid Kind = iota

	InlineHTML
	OpenTag
	OpenTagWithEcho
	CloseTag
	Whitespace
	Comment
	DocComment
	BadCharacter

	Variable
	Identifier
	NameQualified
	NameFullyQualified
	NameRelative
	IntegerLiteral
	FloatLiteral
	ConstantString
	EncapsedAndWhitespace
	StringVarname
	NumString
	StartHeredoc
	EndHeredoc
	DollarOpenCurlyBraces
	CurlyOpen

	Abstract
	And
	Array
	As
	Break
	Callable
	Case
	Catch
	Class
	Clone
	Const
	Continue
	Declare
	Default
	Do
	Echo
	Else
	ElseIf
	Empty
	EndDeclare
	EndFor
	EndForeach
	EndIf
	EndSwitch
	EndWhile
	Enum
	Eval
	Exit
	Extends
	Final
	Finally
	Fn
	For
	Foreach
	Function
	Global
	Goto
	HaltCompiler
	If
	Implements
	Include
	IncludeOnce
	InstanceOf
	InsteadOf
	Interface
	Isset
	List
	Match
	Namespace
	New
	Or
	Print
	Private
	PrivateSet
	Protected
	ProtectedSet
	Public
	PublicSet
	Readonly
	Require
	RequireOnce
	Return
	Static
	Switch
	Throw
	Trait
	Try
	Unset
	Use
	Var
	While
	Xor
	Yield
	YieldFrom

	ClassConst
	DirConst
	FileConst
	FunctionConst
	LineConst
	MethodConst
	NamespaceConst
	PropertyConst
	TraitConst

	IntCast
	FloatCast
	StringCast
	ArrayCast
	ObjectCast
	BoolCast
	UnsetCast
	VoidCast

	Attribute
	PlusEqual
	MinusEqual
	MulEqual
	DivEqual
	ConcatEqual
	ModEqual
	AndEqual
	OrEqual
	XorEqual
	ShiftLeftEqual
	ShiftRightEqual
	CoalesceEqual
	PowEqual
	BooleanOr
	BooleanAnd
	IsEqual
	IsNotEqual
	IsIdentical
	IsNotIdentical
	IsSmallerOrEqual
	IsGreaterOrEqual
	Spaceship
	ShiftLeft
	ShiftRight
	Increment
	Decrement
	ObjectOperator
	NullsafeObjectOperator
	DoubleArrow
	DoubleColon
	NamespaceSeparator
	Ellipsis
	Coalesce
	Pow
	Pipe
	AmpersandFollowedByVarOrVararg
	AmpersandNotFollowedByVarOrVararg

	Semicolon
	Colon
	Comma
	Dot
	LeftBracket
	RightBracket
	LeftParen
	RightParen
	BitwiseOr
	BitwiseXor
	Plus
	Minus
	Slash
	Asterisk
	Equals
	Percent
	Bang
	Tilde
	Dollar
	LessThan
	GreaterThan
	Question
	At
	LeftBrace
	RightBrace
	DoubleQuote
	Backtick
	Ampersand

	kindCount
)

type spec struct {
	php  string
	text string
}

var specs = [kindCount]spec{
	Invalid:               {"T_INVALID", ""},
	InlineHTML:            {"T_INLINE_HTML", ""},
	OpenTag:               {"T_OPEN_TAG", ""},
	OpenTagWithEcho:       {"T_OPEN_TAG_WITH_ECHO", "<?="},
	CloseTag:              {"T_CLOSE_TAG", ""},
	Whitespace:            {"T_WHITESPACE", ""},
	Comment:               {"T_COMMENT", ""},
	DocComment:            {"T_DOC_COMMENT", ""},
	BadCharacter:          {"T_BAD_CHARACTER", ""},
	Variable:              {"T_VARIABLE", ""},
	Identifier:            {"T_STRING", ""},
	NameQualified:         {"T_NAME_QUALIFIED", ""},
	NameFullyQualified:    {"T_NAME_FULLY_QUALIFIED", ""},
	NameRelative:          {"T_NAME_RELATIVE", ""},
	IntegerLiteral:        {"T_LNUMBER", ""},
	FloatLiteral:          {"T_DNUMBER", ""},
	ConstantString:        {"T_CONSTANT_ENCAPSED_STRING", ""},
	EncapsedAndWhitespace: {"T_ENCAPSED_AND_WHITESPACE", ""},
	StringVarname:         {"T_STRING_VARNAME", ""},
	NumString:             {"T_NUM_STRING", ""},
	StartHeredoc:          {"T_START_HEREDOC", ""},
	EndHeredoc:            {"T_END_HEREDOC", ""},
	DollarOpenCurlyBraces: {"T_DOLLAR_OPEN_CURLY_BRACES", "${"},
	CurlyOpen:             {"T_CURLY_OPEN", ""},

	Abstract:     {"T_ABSTRACT", "abstract"},
	And:          {"T_LOGICAL_AND", "and"},
	Array:        {"T_ARRAY", "array"},
	As:           {"T_AS", "as"},
	Break:        {"T_BREAK", "break"},
	Callable:     {"T_CALLABLE", "callable"},
	Case:         {"T_CASE", "case"},
	Catch:        {"T_CATCH", "catch"},
	Class:        {"T_CLASS", "class"},
	Clone:        {"T_CLONE", "clone"},
	Const:        {"T_CONST", "const"},
	Continue:     {"T_CONTINUE", "continue"},
	Declare:      {"T_DECLARE", "declare"},
	Default:      {"T_DEFAULT", "default"},
	Do:           {"T_DO", "do"},
	Echo:         {"T_ECHO", "echo"},
	Else:         {"T_ELSE", "else"},
	ElseIf:       {"T_ELSEIF", "elseif"},
	Empty:        {"T_EMPTY", "empty"},
	EndDeclare:   {"T_ENDDECLARE", "enddeclare"},
	EndFor:       {"T_ENDFOR", "endfor"},
	EndForeach:   {"T_ENDFOREACH", "endforeach"},
	EndIf:        {"T_ENDIF", "endif"},
	EndSwitch:    {"T_ENDSWITCH", "endswitch"},
	EndWhile:     {"T_ENDWHILE", "endwhile"},
	Enum:         {"T_ENUM", ""},
	Eval:         {"T_EVAL", "eval"},
	Exit:         {"T_EXIT", "exit"},
	Extends:      {"T_EXTENDS", "extends"},
	Final:        {"T_FINAL", "final"},
	Finally:      {"T_FINALLY", "finally"},
	Fn:           {"T_FN", "fn"},
	For:          {"T_FOR", "for"},
	Foreach:      {"T_FOREACH", "foreach"},
	Function:     {"T_FUNCTION", "function"},
	Global:       {"T_GLOBAL", "global"},
	Goto:         {"T_GOTO", "goto"},
	HaltCompiler: {"T_HALT_COMPILER", "__halt_compiler"},
	If:           {"T_IF", "if"},
	Implements:   {"T_IMPLEMENTS", "implements"},
	Include:      {"T_INCLUDE", "include"},
	IncludeOnce:  {"T_INCLUDE_ONCE", "include_once"},
	InstanceOf:   {"T_INSTANCEOF", "instanceof"},
	InsteadOf:    {"T_INSTEADOF", "insteadof"},
	Interface:    {"T_INTERFACE", "interface"},
	Isset:        {"T_ISSET", "isset"},
	List:         {"T_LIST", "list"},
	Match:        {"T_MATCH", "match"},
	Namespace:    {"T_NAMESPACE", "namespace"},
	New:          {"T_NEW", "new"},
	Or:           {"T_LOGICAL_OR", "or"},
	Print:        {"T_PRINT", "print"},
	Private:      {"T_PRIVATE", "private"},
	PrivateSet:   {"T_PRIVATE_SET", "private(set)"},
	Protected:    {"T_PROTECTED", "protected"},
	ProtectedSet: {"T_PROTECTED_SET", "protected(set)"},
	Public:       {"T_PUBLIC", "public"},
	PublicSet:    {"T_PUBLIC_SET", "public(set)"},
	Readonly:     {"T_READONLY", "readonly"},
	Require:      {"T_REQUIRE", "require"},
	RequireOnce:  {"T_REQUIRE_ONCE", "require_once"},
	Return:       {"T_RETURN", "return"},
	Static:       {"T_STATIC", "static"},
	Switch:       {"T_SWITCH", "switch"},
	Throw:        {"T_THROW", "throw"},
	Trait:        {"T_TRAIT", "trait"},
	Try:          {"T_TRY", "try"},
	Unset:        {"T_UNSET", "unset"},
	Use:          {"T_USE", "use"},
	Var:          {"T_VAR", "var"},
	While:        {"T_WHILE", "while"},
	Xor:          {"T_LOGICAL_XOR", "xor"},
	Yield:        {"T_YIELD", "yield"},
	YieldFrom:    {"T_YIELD_FROM", ""},

	ClassConst:     {"T_CLASS_C", "__class__"},
	DirConst:       {"T_DIR", "__dir__"},
	FileConst:      {"T_FILE", "__file__"},
	FunctionConst:  {"T_FUNC_C", "__function__"},
	LineConst:      {"T_LINE", "__line__"},
	MethodConst:    {"T_METHOD_C", "__method__"},
	NamespaceConst: {"T_NS_C", "__namespace__"},
	PropertyConst:  {"T_PROPERTY_C", "__property__"},
	TraitConst:     {"T_TRAIT_C", "__trait__"},

	IntCast:    {"T_INT_CAST", ""},
	FloatCast:  {"T_DOUBLE_CAST", ""},
	StringCast: {"T_STRING_CAST", ""},
	ArrayCast:  {"T_ARRAY_CAST", ""},
	ObjectCast: {"T_OBJECT_CAST", ""},
	BoolCast:   {"T_BOOL_CAST", ""},
	UnsetCast:  {"T_UNSET_CAST", ""},
	VoidCast:   {"T_VOID_CAST", ""},

	Attribute:                         {"T_ATTRIBUTE", "#["},
	PlusEqual:                         {"T_PLUS_EQUAL", "+="},
	MinusEqual:                        {"T_MINUS_EQUAL", "-="},
	MulEqual:                          {"T_MUL_EQUAL", "*="},
	DivEqual:                          {"T_DIV_EQUAL", "/="},
	ConcatEqual:                       {"T_CONCAT_EQUAL", ".="},
	ModEqual:                          {"T_MOD_EQUAL", "%="},
	AndEqual:                          {"T_AND_EQUAL", "&="},
	OrEqual:                           {"T_OR_EQUAL", "|="},
	XorEqual:                          {"T_XOR_EQUAL", "^="},
	ShiftLeftEqual:                    {"T_SL_EQUAL", "<<="},
	ShiftRightEqual:                   {"T_SR_EQUAL", ">>="},
	CoalesceEqual:                     {"T_COALESCE_EQUAL", "??="},
	PowEqual:                          {"T_POW_EQUAL", "**="},
	BooleanOr:                         {"T_BOOLEAN_OR", "||"},
	BooleanAnd:                        {"T_BOOLEAN_AND", "&&"},
	IsEqual:                           {"T_IS_EQUAL", "=="},
	IsNotEqual:                        {"T_IS_NOT_EQUAL", "!="},
	IsIdentical:                       {"T_IS_IDENTICAL", "==="},
	IsNotIdentical:                    {"T_IS_NOT_IDENTICAL", "!=="},
	IsSmallerOrEqual:                  {"T_IS_SMALLER_OR_EQUAL", "<="},
	IsGreaterOrEqual:                  {"T_IS_GREATER_OR_EQUAL", ">="},
	Spaceship:                         {"T_SPACESHIP", "<=>"},
	ShiftLeft:                         {"T_SL", "<<"},
	ShiftRight:                        {"T_SR", ">>"},
	Increment:                         {"T_INC", "++"},
	Decrement:                         {"T_DEC", "--"},
	ObjectOperator:                    {"T_OBJECT_OPERATOR", "->"},
	NullsafeObjectOperator:            {"T_NULLSAFE_OBJECT_OPERATOR", "?->"},
	DoubleArrow:                       {"T_DOUBLE_ARROW", "=>"},
	DoubleColon:                       {"T_DOUBLE_COLON", "::"},
	NamespaceSeparator:                {"T_NS_SEPARATOR", "\\"},
	Ellipsis:                          {"T_ELLIPSIS", "..."},
	Coalesce:                          {"T_COALESCE", "??"},
	Pow:                               {"T_POW", "**"},
	Pipe:                              {"T_PIPE", "|>"},
	AmpersandFollowedByVarOrVararg:    {"T_AMPERSAND_FOLLOWED_BY_VAR_OR_VARARG", ""},
	AmpersandNotFollowedByVarOrVararg: {"T_AMPERSAND_NOT_FOLLOWED_BY_VAR_OR_VARARG", ""},

	Semicolon:    {";", ";"},
	Colon:        {":", ":"},
	Comma:        {",", ","},
	Dot:          {".", "."},
	LeftBracket:  {"[", "["},
	RightBracket: {"]", "]"},
	LeftParen:    {"(", "("},
	RightParen:   {")", ")"},
	BitwiseOr:    {"|", "|"},
	BitwiseXor:   {"^", "^"},
	Plus:         {"+", "+"},
	Minus:        {"-", "-"},
	Slash:        {"/", "/"},
	Asterisk:     {"*", "*"},
	Equals:       {"=", "="},
	Percent:      {"%", "%"},
	Bang:         {"!", "!"},
	Tilde:        {"~", "~"},
	Dollar:       {"$", "$"},
	LessThan:     {"<", "<"},
	GreaterThan:  {">", ">"},
	Question:     {"?", "?"},
	At:           {"@", "@"},
	LeftBrace:    {"{", "{"},
	RightBrace:   {"}", "}"},
	DoubleQuote:  {"\"", "\""},
	Backtick:     {"`", "`"},
	Ampersand:    {"&", "&"},
}

func (k Kind) String() string {
	return specs[k].php
}

var introduced = [kindCount]Version{
	NameQualified:                     PHP80,
	NameFullyQualified:                PHP80,
	NameRelative:                      PHP80,
	Match:                             PHP80,
	Attribute:                         PHP80,
	NullsafeObjectOperator:            PHP80,
	Enum:                              PHP81,
	PrivateSet:                        PHP84,
	ProtectedSet:                      PHP84,
	PublicSet:                         PHP84,
	Readonly:                          PHP81,
	PropertyConst:                     PHP84,
	VoidCast:                          PHP85,
	Pipe:                              PHP85,
	AmpersandFollowedByVarOrVararg:    PHP81,
	AmpersandNotFollowedByVarOrVararg: PHP81,
}

func (k Kind) Text() string {
	return specs[k].text
}

func (k Kind) In(v Version) bool {
	return introduced[k] <= v
}

func (k Kind) IsCharacter() bool {
	return specs[k].php == specs[k].text
}

func (k Kind) IsTrivia() bool {
	return k == Whitespace || k == Comment || k == DocComment
}

// IsSemiReserved reports whether k can name a member or a named argument:
// an identifier or any keyword.
func (k Kind) IsSemiReserved() bool {
	return k == Identifier || k >= Abstract && k <= TraitConst
}

var keywords = func() map[string]Kind {
	m := map[string]Kind{"die": Exit}
	for k := Abstract; k <= TraitConst; k++ {
		if text := specs[k].text; text != "" && !strings.ContainsAny(text, "()") {
			m[text] = k
		}
	}
	return m
}()

func Keyword(lowerLabel string) (Kind, bool) {
	k, ok := keywords[lowerLabel]
	return k, ok
}

var punctuators = func() (byFirst [256][]Kind) {
	for n := 3; n > 0; n-- {
		for k := Attribute; k < kindCount; k++ {
			if text := specs[k].text; len(text) == n {
				byFirst[text[0]] = append(byFirst[text[0]], k)
			}
		}
	}
	return byFirst
}()

func Punctuator(src string, v Version) (Kind, int) {
	for _, k := range punctuators[src[0]] {
		if text := specs[k].text; k.In(v) && strings.HasPrefix(src, text) {
			return k, len(text)
		}
	}
	return Invalid, 0
}

var modifierSets = func() map[string]Kind {
	m := map[string]Kind{}
	for k := Abstract; k <= TraitConst; k++ {
		if text := specs[k].text; strings.Contains(text, "(") {
			m[text] = k
		}
	}
	return m
}()

func ModifierSet(lower string) (Kind, bool) {
	k, ok := modifierSets[lower]
	return k, ok
}
