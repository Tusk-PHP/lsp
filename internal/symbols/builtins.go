package symbols

type builtinFunction struct {
	Name   string
	Params []ParamInfo
	Ret    string
	Doc    string
}

var builtinFunctions = []builtinFunction{
	{"array_map", []ParamInfo{{Name: "$callback", Type: "?callable"}, {Name: "$array", Type: "array"}}, "array", "Applies callback to elements"},
	{"array_filter", []ParamInfo{{Name: "$array", Type: "array"}, {Name: "$callback", Type: "?callable"}}, "array", "Filters elements using callback"},
	{"array_reduce", []ParamInfo{{Name: "$array", Type: "array"}, {Name: "$callback", Type: "callable"}, {Name: "$initial", Type: "mixed"}}, "mixed", "Reduces array to single value"},
	{"array_keys", []ParamInfo{{Name: "$array", Type: "array"}}, "array", "Return all keys of an array"},
	{"array_values", []ParamInfo{{Name: "$array", Type: "array"}}, "array", "Return all values of an array"},
	{"array_merge", []ParamInfo{{Name: "$arrays", Type: "array", IsVariadic: true}}, "array", "Merge arrays"},
	{"array_push", []ParamInfo{{Name: "$array", Type: "array", IsReference: true}, {Name: "$values", Type: "mixed", IsVariadic: true}}, "int", "Push onto end of array"},
	{"array_pop", []ParamInfo{{Name: "$array", Type: "array", IsReference: true}}, "mixed", "Pop last element"},
	{"array_unique", []ParamInfo{{Name: "$array", Type: "array"}}, "array", "Remove duplicates"},
	{"array_search", []ParamInfo{{Name: "$needle", Type: "mixed"}, {Name: "$haystack", Type: "array"}}, "int|string|false", "Search for value"},
	{"array_key_exists", []ParamInfo{{Name: "$key", Type: "string|int"}, {Name: "$array", Type: "array"}}, "bool", "Checks if key exists in array"},
	{"array_column", []ParamInfo{{Name: "$array", Type: "array"}, {Name: "$column_key", Type: "int|string|null"}, {Name: "$index_key", Type: "int|string|null"}}, "array", "Return values from a single column"},
	{"in_array", []ParamInfo{{Name: "$needle", Type: "mixed"}, {Name: "$haystack", Type: "array"}}, "bool", "Check if value exists in array"},
	{"count", []ParamInfo{{Name: "$value", Type: "Countable|array"}}, "int", "Count elements"},
	{"sizeof", []ParamInfo{{Name: "$value", Type: "Countable|array"}}, "int", "Alias of count"},
	{"isset", []ParamInfo{{Name: "$var", Type: "mixed"}}, "bool", "Check if variable is set"},
	{"empty", []ParamInfo{{Name: "$var", Type: "mixed"}}, "bool", "Check if variable is empty"},
	{"strlen", []ParamInfo{{Name: "$string", Type: "string"}}, "int", "Get string length"},
	{"str_contains", []ParamInfo{{Name: "$haystack", Type: "string"}, {Name: "$needle", Type: "string"}}, "bool", "Check if string contains substring"},
	{"str_starts_with", []ParamInfo{{Name: "$haystack", Type: "string"}, {Name: "$needle", Type: "string"}}, "bool", "Check if string starts with"},
	{"str_ends_with", []ParamInfo{{Name: "$haystack", Type: "string"}, {Name: "$needle", Type: "string"}}, "bool", "Check if string ends with"},
	{"substr", []ParamInfo{{Name: "$string", Type: "string"}, {Name: "$offset", Type: "int"}}, "string", "Return part of string"},
	{"strpos", []ParamInfo{{Name: "$haystack", Type: "string"}, {Name: "$needle", Type: "string"}}, "int|false", "Find position of substring"},
	{"str_replace", []ParamInfo{{Name: "$search", Type: "string|array"}, {Name: "$replace", Type: "string|array"}, {Name: "$subject", Type: "string|array"}}, "string|array", "Replace occurrences"},
	{"explode", []ParamInfo{{Name: "$separator", Type: "string"}, {Name: "$string", Type: "string"}}, "array", "Split string by separator"},
	{"implode", []ParamInfo{{Name: "$separator", Type: "string"}, {Name: "$array", Type: "array"}}, "string", "Join array elements"},
	{"sprintf", []ParamInfo{{Name: "$format", Type: "string"}, {Name: "$values", Type: "mixed", IsVariadic: true}}, "string", "Return formatted string"},
	{"json_encode", []ParamInfo{{Name: "$value", Type: "mixed"}}, "string|false", "JSON encode a value"},
	{"json_decode", []ParamInfo{{Name: "$json", Type: "string"}, {Name: "$associative", Type: "?bool"}}, "mixed", "Decode JSON string"},
	{"file_get_contents", []ParamInfo{{Name: "$filename", Type: "string"}}, "string|false", "Read entire file"},
	{"file_put_contents", []ParamInfo{{Name: "$filename", Type: "string"}, {Name: "$data", Type: "mixed"}}, "int|false", "Write data to file"},
	{"file_exists", []ParamInfo{{Name: "$filename", Type: "string"}}, "bool", "Check if file exists"},
	{"var_dump", []ParamInfo{{Name: "$value", Type: "mixed", IsVariadic: true}}, "void", "Dump variable info"},
	{"print_r", []ParamInfo{{Name: "$value", Type: "mixed"}, {Name: "$return", Type: "bool"}}, "string|true", "Print human-readable info"},
	{"defined", []ParamInfo{{Name: "$constant_name", Type: "string"}}, "bool", "Checks whether a constant exists"},
	{"define", []ParamInfo{{Name: "$constant_name", Type: "string"}, {Name: "$value", Type: "mixed"}}, "bool", "Defines a named constant"},
	{"class_exists", []ParamInfo{{Name: "$class", Type: "string"}}, "bool", "Check if class is defined"},
	{"get_class", []ParamInfo{{Name: "$object", Type: "?object"}}, "string", "Returns the class name of an object"},
	{"get_called_class", nil, "string", "Returns the late static binding class name"},
	{"get_parent_class", []ParamInfo{{Name: "$object_or_class", Type: "object|string"}}, "string|false", "Retrieves the parent class name"},
	{"is_subclass_of", []ParamInfo{{Name: "$object_or_class", Type: "object|string"}, {Name: "$class", Type: "string"}}, "bool", "Checks if an object or class is a subclass"},
	{"is_a", []ParamInfo{{Name: "$object_or_class", Type: "object|string"}, {Name: "$class", Type: "string"}}, "bool", "Checks object or class type"},
	{"get_class_vars", []ParamInfo{{Name: "$class", Type: "string"}}, "array", "Returns default properties of a class"},
	{"get_object_vars", []ParamInfo{{Name: "$object", Type: "object"}}, "array", "Gets object properties"},
	{"get_class_methods", []ParamInfo{{Name: "$object_or_class", Type: "object|string"}}, "array", "Gets class methods"},
	{"method_exists", []ParamInfo{{Name: "$object_or_class", Type: "object|string"}, {Name: "$method", Type: "string"}}, "bool", "Checks if a method exists"},
	{"property_exists", []ParamInfo{{Name: "$object_or_class", Type: "object|string"}, {Name: "$property", Type: "string"}}, "bool", "Checks if a property exists"},
	{"interface_exists", []ParamInfo{{Name: "$interface", Type: "string"}}, "bool", "Checks if an interface exists"},
	{"trait_exists", []ParamInfo{{Name: "$trait", Type: "string"}}, "bool", "Checks if a trait exists"},
	{"enum_exists", []ParamInfo{{Name: "$enum", Type: "string"}}, "bool", "Checks if an enum exists"},
	{"function_exists", []ParamInfo{{Name: "$function", Type: "string"}}, "bool", "Checks if a function exists"},
	{"preg_match", []ParamInfo{{Name: "$pattern", Type: "string"}, {Name: "$subject", Type: "string"}, {Name: "$matches", Type: "array", IsReference: true}}, "int|false", "Perform regex match"},
	{"preg_replace", []ParamInfo{{Name: "$pattern", Type: "string|array"}, {Name: "$replacement", Type: "string|array"}, {Name: "$subject", Type: "string|array"}}, "string|array|null", "Perform a regular expression search and replace"},
	{"preg_split", []ParamInfo{{Name: "$pattern", Type: "string"}, {Name: "$subject", Type: "string"}}, "array|false", "Split string by a regular expression"},
	{"abs", []ParamInfo{{Name: "$num", Type: "int|float"}}, "int|float", "Absolute value"},
	{"round", []ParamInfo{{Name: "$num", Type: "int|float"}, {Name: "$precision", Type: "int"}}, "float", "Round a float"},
	{"max", []ParamInfo{{Name: "$value", Type: "mixed", IsVariadic: true}}, "mixed", "Find highest value"},
	{"min", []ParamInfo{{Name: "$value", Type: "mixed", IsVariadic: true}}, "mixed", "Find lowest value"},
	{"time", nil, "int", "Return current Unix timestamp"},
	{"date", []ParamInfo{{Name: "$format", Type: "string"}, {Name: "$timestamp", Type: "?int"}}, "string", "Format a local time/date"},
	{"is_string", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if type is string"},
	{"is_int", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if type is int"},
	{"is_bool", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if type is bool"},
	{"is_float", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if type is float"},
	{"is_numeric", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if value is numeric"},
	{"is_object", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if type is object"},
	{"is_callable", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if value can be called as a function"},
	{"is_array", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if type is array"},
	{"is_null", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if variable is null"},
	{"intval", []ParamInfo{{Name: "$value", Type: "mixed"}}, "int", "Get integer value"},
	{"strval", []ParamInfo{{Name: "$value", Type: "mixed"}}, "string", "Get string value"},
	{"trim", []ParamInfo{{Name: "$string", Type: "string"}}, "string", "Strip whitespace"},
	{"strtolower", []ParamInfo{{Name: "$string", Type: "string"}}, "string", "Make lowercase"},
	{"strtoupper", []ParamInfo{{Name: "$string", Type: "string"}}, "string", "Make uppercase"},
	{"class_basename", []ParamInfo{{Name: "$class", Type: "string|object"}}, "string", "Get the class basename of the given object or class"},
}

var builtinClasses = []struct{ Name, Doc string }{
	{"stdClass", "Generic empty class"},
	{"Exception", "Base class for all exceptions"},
	{"DateTime", "Representation of date and time"},
	{"DateTimeImmutable", "Immutable date and time"},
	{"Fiber", "Full-stack interruptible functions (PHP 8.1+)"},
	{"WeakMap", "Maps objects as keys to arbitrary values"},
	{"Generator", "Generator objects returned from generators"},
	{"Closure", "Class used to represent anonymous functions"},
	{"ArrayObject", "Allows objects to work as arrays"},
}

// RegisterBuiltins populates the index with PHP built-in symbols.
func (idx *Index) RegisterBuiltins() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for _, fn := range builtinFunctions {
		idx.addBuiltinFunctionLocked(fn.Name, fn.Params, fn.Ret, fn.Doc)
	}

	for _, name := range generatedBuiltinFunctionNames {
		if idx.symbols[name] != nil {
			continue
		}
		idx.addBuiltinFunctionLocked(name, nil, "mixed", "PHP built-in function")
	}

	for _, cls := range builtinClasses {
		idx.addBuiltinClassLikeLocked(cls.Name, cls.Doc)
	}

	for _, name := range generatedBuiltinClassLikeNames {
		if idx.symbols[name] != nil {
			continue
		}
		idx.addBuiltinClassLikeLocked(name, "PHP built-in class-like symbol")
	}
}

func (idx *Index) addBuiltinFunctionLocked(name string, params []ParamInfo, ret string, doc string) {
	sym := &Symbol{Name: name, FQN: name, Kind: KindFunction, Source: SourceBuiltin, URI: "builtin", ReturnType: ret, DocComment: doc, Params: params}
	idx.symbols[sym.FQN] = sym
	idx.nameIndex[sym.Name] = appendUnique(idx.nameIndex[sym.Name], sym.FQN)
	idx.fileSymbols[sym.URI] = appendUniqueSymbol(idx.fileSymbols[sym.URI], sym)
	idx.sortedNamesDirty = true
	idx.sortedFQNsDirty = true
	idx.namespaceIndex[namespaceForFQN(sym.FQN)] = appendUnique(idx.namespaceIndex[namespaceForFQN(sym.FQN)], sym.FQN)
}

func (idx *Index) addBuiltinClassLikeLocked(name string, doc string) {
	sym := &Symbol{Name: name, FQN: name, Kind: KindClass, Source: SourceBuiltin, URI: "builtin", DocComment: doc}
	idx.symbols[sym.FQN] = sym
	idx.nameIndex[sym.Name] = appendUnique(idx.nameIndex[sym.Name], sym.FQN)
	idx.fileSymbols[sym.URI] = appendUniqueSymbol(idx.fileSymbols[sym.URI], sym)
	idx.sortedNamesDirty = true
	idx.sortedFQNsDirty = true
	idx.namespaceIndex[namespaceForFQN(sym.FQN)] = appendUnique(idx.namespaceIndex[namespaceForFQN(sym.FQN)], sym.FQN)
}

func appendUniqueSymbol(symbols []*Symbol, sym *Symbol) []*Symbol {
	for i, existing := range symbols {
		if existing.FQN == sym.FQN {
			symbols[i] = sym
			return symbols
		}
	}
	return append(symbols, sym)
}
