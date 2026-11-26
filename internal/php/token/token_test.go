package token

import "testing"

func TestEveryKindHasAPHPName(t *testing.T) {
	seen := map[string]Kind{}
	for k := Invalid; k < kindCount; k++ {
		name := k.String()
		if name == "" {
			t.Errorf("kind %d has no PHP name", k)
		}
		if other, dup := seen[name]; dup {
			t.Errorf("%s names both %d and %d", name, other, k)
		}
		seen[name] = k
	}
}

func TestParseVersion(t *testing.T) {
	for in, want := range map[string]Version{"7.4": PHP74, "7.4.33": PHP74, "8.0": PHP80, "8.3.12": PHP83, "8.5": PHP85, "8.6": PHP86} {
		if got, err := ParseVersion(in); err != nil || got != want {
			t.Errorf("%s: got %v, %v", in, got, err)
		}
		if want.String() != in[:3] {
			t.Errorf("%v prints as %s", want, want.String())
		}
	}
	for in, want := range map[string]Version{"7.3": PHP74, "5.6.40": PHP74, "8.7": Latest, "8.10": Latest, "9.0": Latest} {
		if got, err := ParseVersion(in); err != nil || got != want {
			t.Errorf("%s lexes with the closest grammar %v, got %v, %v", in, want, got, err)
		}
	}
	for _, bad := range []string{"8", "8.x", "x.1", "-1.0", "8.-1", "8.256", ""} {
		if _, err := ParseVersion(bad); err == nil {
			t.Errorf("%q must not parse", bad)
		}
	}
	if !Enum.In(PHP81) || Enum.In(PHP80) || !Pipe.In(Latest) || Pipe.In(PHP84) || !Variable.In(PHP74) {
		t.Error("kinds exist from the version that introduced them")
	}
	for _, k := range []Kind{NameQualified, NameFullyQualified, NameRelative, Match, Attribute, NullsafeObjectOperator} {
		if k.In(PHP74) || !k.In(PHP80) {
			t.Errorf("%s arrived in PHP 8.0", k)
		}
	}
	if k, n := Punctuator("?->", PHP74); k != Question || n != 1 {
		t.Errorf("?-> on PHP 7.4: got %v %d, want ? alone", k, n)
	}
}

func TestLookups(t *testing.T) {
	if k, ok := Keyword("function"); !ok || k != Function {
		t.Errorf("function: %v %v", k, ok)
	}
	if k, ok := Keyword("die"); !ok || k != Exit {
		t.Errorf("die: %v %v", k, ok)
	}
	for _, notKeyword := range []string{"private(set)", "enum", "self", "Function"} {
		if _, ok := Keyword(notKeyword); ok {
			t.Errorf("%s is not looked up as a keyword", notKeyword)
		}
	}
	if k, ok := ModifierSet("protected(set)"); !ok || k != ProtectedSet {
		t.Errorf("protected(set): %v %v", k, ok)
	}
	if _, ok := ModifierSet("private"); ok {
		t.Error("private alone is not an asymmetric modifier")
	}
	for src, want := range map[string]struct {
		kind Kind
		n    int
	}{"**=1": {PowEqual, 3}, "**1": {Pow, 2}, "*1": {Asterisk, 1}, "?->x": {NullsafeObjectOperator, 3}, "#[": {Attribute, 2}, "\x00": {Invalid, 0}} {
		if k, n := Punctuator(src, Latest); k != want.kind || n != want.n {
			t.Errorf("%q: got %v %d, want %v %d", src, k, n, want.kind, want.n)
		}
	}
	if PowEqual.Text() != "**=" || Variable.Text() != "" {
		t.Error("fixed texts")
	}
	if !Semicolon.IsCharacter() || Coalesce.IsCharacter() || !Comment.IsTrivia() || Variable.IsTrivia() {
		t.Error("kind classes")
	}
}

func TestIsSemiReserved(t *testing.T) {
	for k, want := range map[Kind]bool{
		Identifier: true, If: true, Class: true, Abstract: true, TraitConst: true, Enum: true,
		Variable: false, NameQualified: false, LeftParen: false, IntCast: false, InlineHTML: false,
	} {
		if got := k.IsSemiReserved(); got != want {
			t.Errorf("%s: IsSemiReserved() = %v, want %v", k, got, want)
		}
	}
}
