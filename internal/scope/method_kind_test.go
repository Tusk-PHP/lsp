package scope

import "testing"

func TestAFunctionIsAMethodOnlyInsideAClassBody(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   map[string]ScopeKind
	}{
		{
			name:   "function after a class",
			source: "<?php\nclass A {}\nfunction f() {}\n",
			want:   map[string]ScopeKind{"f": ScopeFunction},
		},
		{
			name:   "function between two classes",
			source: "<?php\nclass A {\n    public function a() {}\n}\nfunction f() {}\nclass B {\n    public function b() {}\n}\n",
			want:   map[string]ScopeKind{"a": ScopeMethod, "f": ScopeFunction, "b": ScopeMethod},
		},
		{
			name:   "closure after an enum",
			source: "<?php\nenum Suit {\n    case Hearts;\n    public function label() {}\n}\n$fn = function () {};\nfunction f() {}\n",
			want:   map[string]ScopeKind{"label": ScopeMethod, "": ScopeClosure, "f": ScopeFunction},
		},
		{
			name:   "function after an anonymous class",
			source: "<?php\n$o = new class {\n    public function m() {}\n};\nfunction f() {}\n",
			want:   map[string]ScopeKind{"m": ScopeMethod, "f": ScopeFunction},
		},
		{
			name:   "function after an anonymous class with arguments",
			source: "<?php\n$o = new class(1) {\n    public function m() {}\n};\nfunction f() {}\n",
			want:   map[string]ScopeKind{"m": ScopeMethod, "f": ScopeFunction},
		},
		{
			name:   "function after a trait and an interface",
			source: "<?php\ntrait T {\n    public function t() {}\n}\ninterface I {\n    public function i();\n}\nfunction f() {}\n",
			want:   map[string]ScopeKind{"t": ScopeMethod, "i": ScopeMethod, "f": ScopeFunction},
		},
		{
			name:   "function declared inside a method",
			source: "<?php\nclass A {\n    public function m() {\n        function inner() {}\n    }\n}\n",
			want:   map[string]ScopeKind{"m": ScopeMethod, "inner": ScopeFunction},
		},
		{
			name:   "method after a closure in a method",
			source: "<?php\nclass A {\n    public function m() {\n        $fn = function () {};\n    }\n    public function n() {}\n}\nfunction f() {}\n",
			want:   map[string]ScopeKind{"m": ScopeMethod, "": ScopeClosure, "n": ScopeMethod, "f": ScopeFunction},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]ScopeKind{}
			for _, s := range Collect(tc.source).Scopes {
				if s.Kind != ScopeFile {
					got[s.Name] = s.Kind
				}
			}
			if len(got) != len(tc.want) {
				t.Errorf("scopes = %v, want %v", got, tc.want)
			}
			for name, kind := range tc.want {
				if got[name] != kind {
					t.Errorf("scope %q is a %q, want a %q", name, got[name], kind)
				}
			}
		})
	}
}
