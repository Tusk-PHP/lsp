package parser

import (
	"slices"
	"testing"
)

func TestParseDocMethodParams(t *testing.T) {
	type P = DocMethodParam
	tests := []struct {
		name        string
		method      string
		want        []DocMethodParam
		description string
	}{
		{"shape and closure signature", "void configure(array{a: int, b: int} $opts, Closure(A, B): C $cb)",
			[]P{{Type: "array{a: int, b: int}", Name: "$opts"}, {Type: "Closure(A, B): C", Name: "$cb"}}, ""},
		{"generics", "void fill(Collection<int, Post> $items, array<string, list<int>> $map)",
			[]P{{Type: "Collection<int, Post>", Name: "$items"}, {Type: "array<string, list<int>>", Name: "$map"}}, ""},
		{"callable with named and variadic params", "void on(callable(int $a, string ...$rest): void $cb, int $n)",
			[]P{{Type: "callable(int $a, string ...$rest): void", Name: "$cb"}, {Type: "int", Name: "$n"}}, ""},
		{"defaults with commas", "void set(array $x = [1, 2], string $s = 'a,b', string $d = \"c, d\", int $n = 3)",
			[]P{{Type: "array", Name: "$x", Default: "[1, 2]"}, {Type: "string", Name: "$s", Default: "'a,b'"}, {Type: "string", Name: "$d", Default: `"c, d"`}, {Type: "int", Name: "$n", Default: "3"}}, ""},
		{"defaults with keys, calls and match", "void map(array $m = ['a' => 1, 'b' => [2, 3]], $f = foo(1, 2), int $k = match ($v) {1 => 2, default => 3}, $z = 0)",
			[]P{{Type: "array", Name: "$m", Default: "['a' => 1, 'b' => [2, 3]]"}, {Name: "$f", Default: "foo(1, 2)"}, {Type: "int", Name: "$k", Default: "match ($v) {1 => 2, default => 3}"}, {Name: "$z", Default: "0"}}, ""},
		{"comma and parenthesis in a comment in a default", "void c(int $x = 1 /* a, b) */, int $y)",
			[]P{{Type: "int", Name: "$x", Default: "1 /* a, b) */"}, {Type: "int", Name: "$y"}}, ""},
		{"closing parenthesis in a default string", "void wrap(string $s = ')', int $b) Wraps it",
			[]P{{Type: "string", Name: "$s", Default: "')'"}, {Type: "int", Name: "$b"}}, "Wraps it"},
		{"variadics", "void log(string ...$lines, ...$rest)",
			[]P{{Type: "string", Name: "$lines", IsVariadic: true}, {Name: "$rest", IsVariadic: true}}, ""},
		{"variadic without a space", "void log(int...$n)",
			[]P{{Type: "int", Name: "$n", IsVariadic: true}}, ""},
		{"by reference", "void fill(array &$out, int& $n, &$m, string &...$refs, int&$tight)",
			[]P{{Type: "array", Name: "$out", IsReference: true}, {Type: "int", Name: "$n", IsReference: true}, {Name: "$m", IsReference: true}, {Type: "string", Name: "$refs", IsReference: true, IsVariadic: true}, {Type: "int", Name: "$tight", IsReference: true}}, ""},
		{"intersection and union types", "void both(A & B $x, A&B $y, int|null $z, ?int $w = null)",
			[]P{{Type: "A & B", Name: "$x"}, {Type: "A&B", Name: "$y"}, {Type: "int|null", Name: "$z"}, {Type: "?int", Name: "$w", Default: "null"}}, ""},
		{"names without types", "void pair($a, $b)",
			[]P{{Name: "$a"}, {Name: "$b"}}, ""},
		{"types without names", "void pair(int, string)",
			[]P{{Type: "int"}, {Type: "string"}}, ""},
		{"spaces and a trailing comma", "void spaced( int $a , )",
			[]P{{Type: "int", Name: "$a"}}, ""},
		{"text after a name is skipped", "void note(int $a the value, int $b)",
			[]P{{Type: "int", Name: "$a"}, {Type: "int", Name: "$b"}}, ""},
		{"quoted literal type", `void quote('a, b' $x, 'it\'s' $y)`,
			[]P{{Type: "'a, b'", Name: "$x"}, {Type: `'it\'s'`, Name: "$y"}}, ""},
		{"empty list", "void none() Does nothing", nil, "Does nothing"},
		{"unclosed list", "void open(int $a, int $b", nil, ""},
		{"unclosed default", "void open(array $a = [1, 2", nil, ""},
		{"stray closing brace", "void stray(int $a }, int $b)", nil, ""},
		{"stray closing bracket in a type", "void stray(int] $a)", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := ParseDocBlock("/**\n * @method " + tt.method + "\n */")
			if len(doc.Methods) != 1 {
				t.Fatalf("got %d methods, want 1", len(doc.Methods))
			}
			m := doc.Methods[0]
			if !slices.Equal(m.Params, tt.want) {
				t.Errorf("params:\n got %+v\nwant %+v", m.Params, tt.want)
			}
			if m.Description != tt.description {
				t.Errorf("description = %q, want %q", m.Description, tt.description)
			}
		})
	}
}

func TestParseDocMethodReturnTypes(t *testing.T) {
	tests := []struct {
		method      string
		returnType  string
		name        string
		params      []DocMethodParam
		description string
		static      bool
	}{
		{"Closure(int): void make()", "Closure(int): void", "make", nil, "", false},
		{"Closure(int) make(string $s)", "Closure(int)", "make", []DocMethodParam{{Type: "string", Name: "$s"}}, "", false},
		{"static callable(int, string): bool handler() Gets it", "callable(int, string): bool", "handler", nil, "Gets it", true},
		{"\\Closure(): (int|string) pick()", "\\Closure(): (int|string)", "pick", nil, "", false},
		{"Closure<T>(T): T identity()", "Closure<T>(T): T", "identity", nil, "", false},
		{"static Collection<int, Post> all()", "Collection<int, Post>", "all", nil, "", true},
		{"array{id: int, tags: list<string>} meta()", "array{id: int, tags: list<string>}", "meta", nil, "", false},
		{"int | null find(int $id)", "int | null", "find", []DocMethodParam{{Type: "int", Name: "$id"}}, "", false},
		{"Foo bar (int $x)", "Foo", "bar", []DocMethodParam{{Type: "int", Name: "$x"}}, "", false},
		{"foo() see(x)", "", "foo", nil, "see(x)", false},
		{"foo (int $x)", "", "foo", []DocMethodParam{{Type: "int", Name: "$x"}}, "", false},
		{"callable()", "", "callable", nil, "", false},
		{"static make()", "", "make", nil, "", true},
		{"Collection getCollection  the items", "Collection", "getCollection", nil, "the items", false},
		{"getName", "", "getName", nil, "", false},
		{"Foo<int>", "", "", nil, "", false},
		{"static", "", "static", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			m := parseDocMethod(tt.method)
			if m.ReturnType != tt.returnType || m.Name != tt.name || m.Description != tt.description || m.IsStatic != tt.static || !slices.Equal(m.Params, tt.params) {
				t.Fatalf("parseDocMethod(%q) = %+v", tt.method, m)
			}
			if (m.ParsedType != nil) != (tt.returnType != "") {
				t.Fatalf("parsed type = %+v for return type %q", m.ParsedType, tt.returnType)
			}
		})
	}
}
