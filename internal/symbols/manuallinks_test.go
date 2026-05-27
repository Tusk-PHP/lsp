package symbols

import (
	"testing"
)

func TestPHPManualURL(t *testing.T) {
	tests := []struct {
		name     string
		sym      *Symbol
		owner    *Symbol
		locale   string
		expected string
	}{
		// --- original tests translated ---
		{
			name:     "builtin function",
			sym:      &Symbol{Name: "array_map", Kind: KindFunction, URI: "builtin", Source: SourceBuiltin},
			expected: "https://www.php.net/manual/en/function.array-map.php",
		},
		{
			name:     "builtin class",
			sym:      &Symbol{Name: "DateTime", Kind: KindClass, URI: "builtin", Source: SourceBuiltin},
			expected: "https://www.php.net/manual/en/class.datetime.php",
		},
		{
			name:     "user-defined function",
			sym:      &Symbol{Name: "myFunc", Kind: KindFunction, URI: "file:///app.php", Source: SourceProject},
			expected: "",
		},
		{
			name:     "user-defined class",
			sym:      &Symbol{Name: "MyClass", Kind: KindClass, URI: "file:///app.php", Source: SourceProject},
			expected: "",
		},
		{
			name:     "builtin method - no owner",
			sym:      &Symbol{Name: "format", Kind: KindMethod, URI: "builtin", Source: SourceBuiltin},
			expected: "",
		},
		{
			name:     "function with underscores",
			sym:      &Symbol{Name: "str_starts_with", Kind: KindFunction, URI: "builtin", Source: SourceBuiltin},
			expected: "https://www.php.net/manual/en/function.str-starts-with.php",
		},
		{
			name:     "builtin stub function",
			sym:      &Symbol{Name: "array_reverse", Kind: KindFunction, Source: SourceBuiltin, URI: "builtin://php/core.php"},
			expected: "https://www.php.net/manual/en/function.array-reverse.php",
		},

		// --- new cases per kind ---
		{
			name:     "namespaced class",
			sym:      &Symbol{Name: "Engine", FQN: "Random\\Engine", Kind: KindClass, Source: SourceBuiltin, URI: "builtin"},
			expected: "https://www.php.net/manual/en/class.random-engine.php",
		},
		{
			name:  "method",
			sym:   &Symbol{Name: "getName", Kind: KindMethod, Source: SourceBuiltin},
			owner: &Symbol{Name: "ReflectionClass", FQN: "ReflectionClass", Kind: KindClass},
			expected: "https://www.php.net/manual/en/reflectionclass.getname.php",
		},
		{
			name:  "magic method",
			sym:   &Symbol{Name: "__construct", Kind: KindMethod, Source: SourceBuiltin},
			owner: &Symbol{Name: "DateTime", FQN: "DateTime"},
			expected: "https://www.php.net/manual/en/datetime.construct.php",
		},
		{
			name:  "namespaced method",
			sym:   &Symbol{Name: "generate", Kind: KindMethod, Source: SourceBuiltin},
			owner: &Symbol{Name: "Engine", FQN: "Random\\Engine"},
			expected: "https://www.php.net/manual/en/random-engine.generate.php",
		},
		{
			name:  "property",
			sym:   &Symbol{Name: "message", Kind: KindProperty, Source: SourceBuiltin},
			owner: &Symbol{Name: "Exception", FQN: "Exception"},
			expected: "https://www.php.net/manual/en/class.exception.php#exception.props.message",
		},
		{
			name:  "class constant",
			sym:   &Symbol{Name: "ATOM", Kind: KindConstant, Source: SourceBuiltin},
			owner: &Symbol{Name: "DateTimeInterface", FQN: "DateTimeInterface"},
			expected: "https://www.php.net/manual/en/class.datetimeinterface.php#datetimeinterface.constants.atom",
		},
		{
			name:     "predefined constant from extension",
			sym:      &Symbol{Name: "JSON_THROW_ON_ERROR", Kind: KindConstant, Source: SourceBuiltin, URI: "builtin://php/extensions/json/php-7.4.php"},
			expected: "https://www.php.net/manual/en/json.constants.php#constant.json-throw-on-error",
		},
		{
			name:     "predefined constant from core",
			sym:      &Symbol{Name: "PHP_INT_MAX", Kind: KindConstant, Source: SourceBuiltin, URI: "builtin://php/core/php-7.4.php"},
			expected: "https://www.php.net/manual/en/reserved.constants.php#constant.php-int-max",
		},
		{
			name:     "method without owner",
			sym:      &Symbol{Name: "someMethod", Kind: KindMethod, Source: SourceBuiltin},
			expected: "",
		},
		{
			name:     "property without owner",
			sym:      &Symbol{Name: "someProperty", Kind: KindProperty, Source: SourceBuiltin},
			expected: "",
		},
		{
			name:     "unknown locale falls back to en",
			sym:      &Symbol{Name: "array_map", Kind: KindFunction, Source: SourceBuiltin, URI: "builtin"},
			locale:   "xx",
			expected: "https://www.php.net/manual/en/function.array-map.php",
		},
		{
			name:     "known locale es",
			sym:      &Symbol{Name: "array_map", Kind: KindFunction, Source: SourceBuiltin, URI: "builtin"},
			locale:   "es",
			expected: "https://www.php.net/manual/es/function.array-map.php",
		},
		{
			name:     "non-builtin",
			sym:      &Symbol{Name: "x", Kind: KindFunction, Source: SourceProject},
			expected: "",
		},

		// --- leading-backslash FQN cases (Bug 1) ---
		{
			name:     "class with leading backslash",
			sym:      &Symbol{Name: "ReflectionMethod", FQN: "\\ReflectionMethod", Kind: KindClass, Source: SourceBuiltin, URI: "builtin"},
			expected: "https://www.php.net/manual/en/class.reflectionmethod.php",
		},
		{
			name:     "namespaced class with leading backslash",
			sym:      &Symbol{Name: "Engine", FQN: "\\Random\\Engine", Kind: KindClass, Source: SourceBuiltin, URI: "builtin"},
			expected: "https://www.php.net/manual/en/class.random-engine.php",
		},
		{
			name:  "method whose owner FQN has leading backslash",
			sym:   &Symbol{Name: "getName", Kind: KindMethod, Source: SourceBuiltin},
			owner: &Symbol{Name: "ReflectionClass", FQN: "\\ReflectionClass"},
			expected: "https://www.php.net/manual/en/reflectionclass.getname.php",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PHPManualURL(tt.sym, tt.owner, tt.locale)
			if got != tt.expected {
				t.Errorf("PHPManualURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}
