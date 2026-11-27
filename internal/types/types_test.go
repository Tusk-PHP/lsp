package types

import "testing"

func TestParseArrayShapeBasic(t *testing.T) {
	fields := ParseArrayShape("array{name: string, age: int}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Key != "name" || fields[0].Type != "string" {
		t.Errorf("field 0: got %+v", fields[0])
	}
	if fields[1].Key != "age" || fields[1].Type != "int" {
		t.Errorf("field 1: got %+v", fields[1])
	}
}

func TestParseArrayShapeOptional(t *testing.T) {
	fields := ParseArrayShape("array{name: string, address?: string}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Optional {
		t.Error("name should not be optional")
	}
	if !fields[1].Optional || fields[1].Key != "address" {
		t.Errorf("address should be optional, got %+v", fields[1])
	}
}

func TestParseArrayShapeNested(t *testing.T) {
	fields := ParseArrayShape("array{user: array{name: string, age: int}, count: int}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Type != "array{name: string, age: int}" {
		t.Errorf("unexpected nested field: %+v", fields[0])
	}
}

func TestParseArrayShapeInUnion(t *testing.T) {
	fields := ParseArrayShape("string|array{key: int}|null")
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if fields[0].Key != "key" || fields[0].Type != "int" {
		t.Errorf("got %+v", fields[0])
	}
}

func TestParseArrayShapeQuotedKeys(t *testing.T) {
	fields := ParseArrayShape("array{'content-type': string, 'x-api-key': string}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Key != "content-type" {
		t.Errorf("expected unquoted key, got %q", fields[0].Key)
	}
}

func TestParseTypeStructured(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kind  TypeKind
		out   string
	}{
		{"union", "Foo|Bar|null", TypeKindUnion, "Foo | Bar | null"},
		{"intersection", "Foo&Bar", TypeKindIntersection, "Foo & Bar"},
		{"generic", "Collection<int, User>", TypeKindGeneric, "Collection<int, User>"},
		{"array suffix", "User[]", TypeKindArray, "User[]"},
		{"callable", "callable(string, int): bool", TypeKindCallable, "callable(string, int): bool"},
		{"generic callable", "callable<T of array<int, User>, U>(T, U): T", TypeKindCallable, "callable<T of array<int, User>, U>(T, U): T"},
		{"bare callable", "callable", TypeKindCallable, "callable"},
		{"callable with a quoted bracket", "callable('a)', \"b>\"): void", TypeKindCallable, "callable('a)', \"b>\"): void"},
		{"generic callable with a quoted bracket", "callable<T of 'a>'>(T): T", TypeKindCallable, "callable<T of 'a>'>(T): T"},
		{"generic closure followed by text", "Closure<T>x", TypeKindNamed, "Closure<T>x"},
		{"unclosed callable", "callable(int", TypeKindNamed, "callable(int"},
		{"generic closure without a signature", "Closure<T>", TypeKindGeneric, "Closure<T>"},
		{"unclosed generic closure", "\\Closure<T(T): T", TypeKindNamed, "Closure<T(T): T"},
		{"literal", "'ok'", TypeKindLiteral, "'ok'"},
		{"conditional", "($input is class-string<T> ? T : object)", TypeKindConditional, "($input is class-string<T> ? T : object)"},
		{"object shape", "object{name: string, age?: int}", TypeKindShape, "object{name: string, age?: int}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := Parse(tt.input)
			if parsed == nil {
				t.Fatal("expected parsed type")
			}
			if parsed.Kind != tt.kind {
				t.Fatalf("kind = %v, want %v", parsed.Kind, tt.kind)
			}
			if got := parsed.String(); got != tt.out {
				t.Fatalf("String() = %q, want %q", got, tt.out)
			}
		})
	}
}

func TestExpandAliases(t *testing.T) {
	aliases := map[string]*Type{
		"Payload": Parse("array{id: int, user: User}"),
		"Result":  Parse("Payload|false"),
	}

	expanded := ExpandAliases(Parse("Result"), aliases)
	if expanded == nil {
		t.Fatal("expected expanded type")
	}
	if got := expanded.String(); got != "array{id: int, user: User} | false" {
		t.Fatalf("String() = %q", got)
	}
}

func TestExtractDocTypeString(t *testing.T) {
	tests := []struct {
		input    string
		wantType string
		wantRest string
	}{
		{"string $name", "string", "$name"},
		{"array{name: string, age: int} $config desc", "array{name: string, age: int}", "$config desc"},
		{"Collection<User> $users", "Collection<User>", "$users"},
		{"callable(string, int): bool $handler", "callable(string, int): bool", "$handler"},
		{"($input is class-string<T> ? T : object) description", "($input is class-string<T> ? T : object)", "description"},
		{"array<int, string>[] $rows", "array<int, string>[]", "$rows"},
		{"T['a b'] $v", "T['a b']", "$v"},
		{"", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			typ, rest := ExtractDocTypeString(tt.input)
			if typ != tt.wantType {
				t.Errorf("type = %q, want %q", typ, tt.wantType)
			}
			if rest != tt.wantRest {
				t.Errorf("rest = %q, want %q", rest, tt.wantRest)
			}
		})
	}
}

func TestExtractParamListType(t *testing.T) {
	tests := []struct {
		input    string
		wantType string
		wantRest string
	}{
		{"string $name, int $n)", "string", "$name, int $n)"},
		{"array{a: int, b: int} $opts)", "array{a: int, b: int}", "$opts)"},
		{"Closure(A, B): C $cb)", "Closure(A, B): C", "$cb)"},
		{"array<int, list<int>> $m", "array<int, list<int>>", "$m"},
		{"'a, b' $x", "'a, b'", "$x"},
		{`'it\'s, ok' $x`, `'it\'s, ok'`, "$x"},
		{"int, string)", "int", ", string)"},
		{"int)", "int", ")"},
		{"int &$x", "int", "&$x"},
		{"int& $x", "int", "& $x"},
		{"int&$x", "int", "&$x"},
		{"string &...$xs", "string", "&...$xs"},
		{"int...$xs", "int", "...$xs"},
		{"A & B $x", "A & B", "$x"},
		{"A&B $x", "A&B", "$x"},
		{"A & $x", "A", "& $x"},
		{"A &", "A &", ""},
		{"int[] $xs", "int[]", "$xs"},
		{"int] $x", "int", "] $x"},
		{"1.5 $x", "1.5", "$x"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			typ, rest := ExtractParamListType(tt.input)
			if typ != tt.wantType {
				t.Errorf("type = %q, want %q", typ, tt.wantType)
			}
			if rest != tt.wantRest {
				t.Errorf("rest = %q, want %q", rest, tt.wantRest)
			}
		})
	}
}
