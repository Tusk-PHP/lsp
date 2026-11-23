package rule

import (
	"fmt"
	"slices"
)

type Code string

const (
	SyntaxError           Code = "syntax-error"
	AbstractInConcrete    Code = "abstract-in-concrete"
	UnknownClass          Code = "unknown-class"
	UnknownFunction       Code = "unknown-function"
	UnknownMember         Code = "unknown-member"
	BuiltinUnavailable    Code = "builtin-unavailable"
	UnusedImport          Code = "unused-import"
	UnusedPrivateMethod   Code = "unused-private-method"
	UnusedPrivateProperty Code = "unused-private-property"
	UnreachableCode       Code = "unreachable-code"
	RedundantUnionMember  Code = "redundant-union-member"
	RedundantNullsafe     Code = "redundant-nullsafe"
	UndefinedVariable     Code = "undefined-variable"
	UnusedVariable        Code = "unused-variable"
	Deprecated            Code = "deprecated"
	UnusedResult          Code = "unused-result"
	InvalidOverride       Code = "invalid-override"
	ArgumentCountMismatch Code = "argument-count-mismatch"
	UnknownColumn         Code = "unknown-column"
	UnknownRelation       Code = "unknown-relation"
	UnknownView           Code = "unknown-view"
	UnknownComponent      Code = "unknown-component"
	UnclosedDirective     Code = "unclosed-directive"
)

var All = []Code{
	SyntaxError,
	AbstractInConcrete,
	UnknownClass,
	UnknownFunction,
	UnknownMember,
	BuiltinUnavailable,
	UnusedImport,
	UnusedPrivateMethod,
	UnusedPrivateProperty,
	UnreachableCode,
	RedundantUnionMember,
	RedundantNullsafe,
	UndefinedVariable,
	UnusedVariable,
	Deprecated,
	UnusedResult,
	InvalidOverride,
	ArgumentCountMismatch,
	UnknownColumn,
	UnknownRelation,
	UnknownView,
	UnknownComponent,
	UnclosedDirective,
}

func ParseCode(s string) (Code, error) {
	code := Code(s)
	if !slices.Contains(All, code) {
		return "", fmt.Errorf("unknown diagnostic rule %q", s)
	}
	return code, nil
}

type Severity int

const (
	SeverityError   Severity = 1
	SeverityWarning Severity = 2
	SeverityInfo    Severity = 3
	SeverityHint    Severity = 4
)

var Severities = []Severity{SeverityError, SeverityWarning, SeverityInfo, SeverityHint}

var severityNames = map[Severity]string{
	SeverityError:   "error",
	SeverityWarning: "warning",
	SeverityInfo:    "info",
	SeverityHint:    "hint",
}

func (s Severity) String() string {
	return severityNames[s]
}

func ParseSeverity(s string) (Severity, error) {
	for severity, name := range severityNames {
		if name == s {
			return severity, nil
		}
	}
	return 0, fmt.Errorf("unknown severity %q (want error, warning, info or hint)", s)
}

type Source string

const (
	SourceTusk    Source = "tusk-php"
	SourcePHPStan Source = "phpstan"
	SourcePint    Source = "pint"
)
