package parser

import (
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

// Scalars is a set of PHP's scalar and array types, the values an operator
// can be typed over. The zero value is unknown: a value that may be an object
// or anything else an operator can't be typed for.
type Scalars uint8

// The members of a Scalars set.
const (
	ScalarInt Scalars = 1 << iota
	ScalarFloat
	ScalarString
	ScalarBool
	ScalarNull
	ScalarArray
)

const scalarKinds = 6

var scalarNames = [scalarKinds]string{"int", "float", "string", "bool", "null", "array"}

var scalarStrings = func() [1 << scalarKinds]string {
	var out [1 << scalarKinds]string
	for s := range out {
		var b strings.Builder
		for i, name := range scalarNames {
			if s&(1<<i) == 0 {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('|')
			}
			b.WriteString(name)
		}
		out[s] = b.String()
	}
	return out
}()

// String is the set as a PHPDoc union, as "int|float"; empty when unknown.
func (s Scalars) String() string {
	return scalarStrings[s&(1<<scalarKinds-1)]
}

// Names are the type names in the set, in the order String writes them.
func (s Scalars) Names() []string {
	names := make([]string, 0, scalarKinds)
	for i, name := range scalarNames {
		if s&(1<<i) != 0 {
			names = append(names, name)
		}
	}
	return names
}

// ScalarOf is the set a single type name stands for: int and its refinements
// such as positive-int, string and its refinements, bool, true and false,
// null, array and list types, and the unions numeric, scalar and array-key.
// It is unknown for any other name.
func ScalarOf(name string) Scalars {
	switch name {
	case "int", "positive-int", "negative-int", "non-positive-int", "non-negative-int", "non-zero-int", "literal-int":
		return ScalarInt
	case "float":
		return ScalarFloat
	case "string", "non-empty-string", "non-falsy-string", "truthy-string", "numeric-string", "literal-string",
		"non-empty-literal-string", "lowercase-string", "uppercase-string", "non-empty-lowercase-string",
		"non-empty-uppercase-string", "class-string", "interface-string", "trait-string", "enum-string", "callable-string":
		return ScalarString
	case "bool", "true", "false":
		return ScalarBool
	case "null":
		return ScalarNull
	case "array", "list", "non-empty-array", "non-empty-list":
		return ScalarArray
	case "numeric":
		return ScalarInt | ScalarFloat | ScalarString
	case "scalar":
		return ScalarInt | ScalarFloat | ScalarString | ScalarBool
	case "array-key":
		return ScalarInt | ScalarString
	}
	return 0
}

// OperatorType is the type a PHP operator expression evaluates to: a binary
// operator other than ?? and |>, a prefix !, -, + or ~, or instanceof.
// operand types an operand; it is called only when the result depends on the
// operands. The result is unknown when e is no such expression, when an
// operand is unknown, or when PHP would throw a TypeError for the operands,
// as for an array in arithmetic or an object, which an extension such as GMP
// may overload the operator for. Integer arithmetic is int, as PHPStan types
// it; it doesn't follow PHP into float on overflow, except where ** is given
// two integer literals.
func OperatorType(e ast.Expr, src string, operand func(ast.Expr) Scalars) Scalars {
	switch n := e.(type) {
	case *ast.InstanceOf:
		return ScalarBool
	case *ast.Unary:
		return unaryType(n, operand)
	case *ast.Binary:
		return binaryType(n, src, operand)
	}
	return 0
}

func unaryType(n *ast.Unary, operand func(ast.Expr) Scalars) Scalars {
	switch n.Op {
	case token.Bang:
		return ScalarBool
	case token.Minus, token.Plus:
		return eachOf(operand(n.Operand), negated)
	case token.Tilde:
		return eachOf(operand(n.Operand), complemented)
	}
	return 0
}

func negated(s Scalars) Scalars {
	switch s {
	case ScalarInt, ScalarBool, ScalarNull:
		return ScalarInt
	case ScalarFloat:
		return ScalarFloat
	case ScalarString:
		return ScalarInt | ScalarFloat
	}
	return 0
}

func complemented(s Scalars) Scalars {
	switch s {
	case ScalarInt, ScalarFloat:
		return ScalarInt
	case ScalarString:
		return ScalarString
	}
	return 0
}

func binaryType(n *ast.Binary, src string, operand func(ast.Expr) Scalars) Scalars {
	switch n.Op {
	case token.BooleanAnd, token.BooleanOr, token.And, token.Or, token.Xor,
		token.IsEqual, token.IsNotEqual, token.IsIdentical, token.IsNotIdentical,
		token.LessThan, token.IsSmallerOrEqual, token.GreaterThan, token.IsGreaterOrEqual:
		return ScalarBool
	case token.Spaceship:
		return ScalarInt
	case token.Dot:
		return ScalarString
	case token.Plus:
		return pairsOf(operand(n.Left), operand(n.Right), added)
	case token.Minus, token.Asterisk:
		return pairsOf(operand(n.Left), operand(n.Right), arithmetic)
	case token.Slash:
		return pairsOf(operand(n.Left), operand(n.Right), divided)
	case token.Percent, token.ShiftLeft, token.ShiftRight:
		return pairsOf(operand(n.Left), operand(n.Right), integral)
	case token.Ampersand, token.AmpersandFollowedByVarOrVararg, token.AmpersandNotFollowedByVarOrVararg,
		token.BitwiseOr, token.BitwiseXor:
		return pairsOf(operand(n.Left), operand(n.Right), bitwise)
	case token.Pow:
		exponent := powExponent(n, src)
		return pairsOf(operand(n.Left), operand(n.Right), func(l, r Scalars) Scalars {
			return powered(l, r, exponent)
		})
	}
	return 0
}

func eachOf(s Scalars, f func(Scalars) Scalars) Scalars {
	if s == 0 {
		return 0
	}
	var out Scalars
	for part := Scalars(1); part < 1<<scalarKinds; part <<= 1 {
		if s&part == 0 {
			continue
		}
		r := f(part)
		if r == 0 {
			return 0
		}
		out |= r
	}
	return out
}

func pairsOf(left, right Scalars, f func(l, r Scalars) Scalars) Scalars {
	return eachOf(left, func(l Scalars) Scalars {
		return eachOf(right, func(r Scalars) Scalars { return f(l, r) })
	})
}

func numeric(l, r Scalars) bool {
	return l != ScalarArray && r != ScalarArray
}

func added(l, r Scalars) Scalars {
	if l == ScalarArray && r == ScalarArray {
		return ScalarArray
	}
	return arithmetic(l, r)
}

func arithmetic(l, r Scalars) Scalars {
	switch {
	case !numeric(l, r):
		return 0
	case l == ScalarFloat || r == ScalarFloat:
		return ScalarFloat
	case l == ScalarString || r == ScalarString:
		return ScalarInt | ScalarFloat
	}
	return ScalarInt
}

func divided(l, r Scalars) Scalars {
	if t := arithmetic(l, r); t != ScalarInt {
		return t
	}
	return ScalarInt | ScalarFloat
}

func integral(l, r Scalars) Scalars {
	if !numeric(l, r) {
		return 0
	}
	return ScalarInt
}

func bitwise(l, r Scalars) Scalars {
	switch {
	case !numeric(l, r):
		return 0
	case l == ScalarString && r == ScalarString:
		return ScalarString
	}
	return ScalarInt
}

type exponentKind uint8

const (
	exponentUnknown exponentKind = iota
	exponentNegative
	exponentNonNegative
	exponentOverflows
)

func powered(l, r Scalars, exponent exponentKind) Scalars {
	t := arithmetic(l, r)
	if t != ScalarInt || r != ScalarInt {
		return t
	}
	switch exponent {
	case exponentNonNegative:
		return ScalarInt
	case exponentNegative, exponentOverflows:
		return ScalarFloat
	}
	return ScalarInt | ScalarFloat
}

func powExponent(n *ast.Binary, src string) exponentKind {
	exp, ok := intLiteralValue(n.Right, src)
	switch {
	case !ok:
		return exponentUnknown
	case exp < 0:
		return exponentNegative
	}
	base, ok := intLiteralValue(n.Left, src)
	if ok && powOverflows(base, exp) {
		return exponentOverflows
	}
	return exponentNonNegative
}

func intLiteralValue(e ast.Expr, src string) (int64, bool) {
	sign := int64(1)
	if u, ok := e.(*ast.Unary); ok && (u.Op == token.Minus || u.Op == token.Plus) {
		if u.Op == token.Minus {
			sign = -1
		}
		e = u.Operand
	}
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != token.IntegerLiteral {
		return 0, false
	}
	v, err := strconv.ParseInt(lit.Text(src), 0, 64)
	if err != nil {
		return 0, false
	}
	return sign * v, true
}

func powOverflows(base, exp int64) bool {
	if base >= -1 && base <= 1 {
		return false
	}
	result := int64(1)
	for ; exp > 0; exp-- {
		next := result * base
		if next/base != result {
			return true
		}
		result = next
	}
	return false
}
