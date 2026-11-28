package symbols

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func builtinDocFor(t *testing.T, version, name string) *parser.DocBlock {
	t.Helper()
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: version})
	sym := idx.Lookup(name)
	if sym == nil {
		t.Fatalf("%s isn't registered on PHP %s", name, version)
	}
	if doc := parser.ParseDocBlock(sym.DocComment); doc != nil {
		return doc
	}
	return &parser.DocBlock{}
}

func TestBuiltinFunctionsCarryPHPDocTypesByVersion(t *testing.T) {
	cases := []struct {
		version, name, ret string
		params             map[string]string
	}{
		{"7.4", "explode", "list<string>|false", nil},
		{"8.0", "explode", "list<string>", map[string]string{"$separator": "non-empty-string"}},
		{"8.4", "explode", "list<string>", map[string]string{"$separator": "non-empty-string"}},
		{"7.4", "array_keys", "list<array-key>", nil},
		{"8.4", "array_keys", "list<array-key>", nil},
		{"7.4", "str_split", "non-empty-list<string>|false", nil},
		{"8.1", "str_split", "non-empty-list<string>", map[string]string{"$length": "positive-int"}},
		{"8.2", "str_split", "list<string>", map[string]string{"$length": "positive-int"}},
		{"8.0", "strcmp", "", nil},
	}
	for _, c := range cases {
		doc := builtinDocFor(t, c.version, c.name)
		if doc.Return.Type != c.ret {
			t.Errorf("%s on PHP %s returns %q, want %q", c.name, c.version, doc.Return.Type, c.ret)
		}
		got := map[string]string{}
		for _, p := range doc.Params {
			got[p.Name] = p.Type
		}
		if len(got) != len(c.params) {
			t.Errorf("%s on PHP %s has PHPDoc params %v, want %v", c.name, c.version, got, c.params)
		}
		for name, typ := range c.params {
			if got[name] != typ {
				t.Errorf("%s on PHP %s: %s is %q, want %q", c.name, c.version, name, got[name], typ)
			}
		}
	}
}

func TestBuiltinMethodsCarryPHPDocTypesByVersion(t *testing.T) {
	cases := []struct{ version, name, ret string }{
		{"7.4", "ArrayObject::count", "0|positive-int"},
		{"8.4", "ArrayObject::count", "0|positive-int"},
		{"8.4", "DateTimeZone::listIdentifiers", "list<string>"},
		{"8.4", "Exception::getTrace", "list<array{function:string,line?:int,file?:string,class?:class-string,type?:'::'|'->',args?:list<mixed>,object?:object}>"},
		{"8.2", "ReflectionClass::getStaticProperties", ""},
		{"8.3", "ReflectionClass::getStaticProperties", "array<string, mixed>"},
		{"7.4", "Phar::copy", "bool"},
		{"8.4", "Phar::copy", ""},
		{"8.4", "ReflectionClass::getName", "class-string<T>"},
		{"8.4", "PDOStatement::rowCount", ""},
	}
	for _, c := range cases {
		if got := builtinDocFor(t, c.version, c.name).Return.Type; got != c.ret {
			t.Errorf("%s on PHP %s returns %q, want %q", c.name, c.version, got, c.ret)
		}
	}
}

func TestBuiltinFunctionsHaveTheirNativeSignatures(t *testing.T) {
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "8.4"})
	sym := idx.Lookup("array_chunk")
	if sym == nil || sym.ReturnType != "array" || len(sym.Params) != 3 || sym.Params[0].Name != "$array" || sym.Params[2].DefaultValue != "false" {
		t.Fatalf("array_chunk should have its reflected signature, got %+v", sym)
	}
}

func TestGeneratedBuiltinDocsParse(t *testing.T) {
	for i := range generatedBuiltinFunctionSignatures {
		sig := &generatedBuiltinFunctionSignatures[i]
		checkGeneratedDocs(t, sig.Name, sig.Params, sig.Docs)
	}
	for _, c := range generatedBuiltinClassLikes {
		for _, m := range c.Members {
			checkGeneratedDocs(t, c.Name+"::"+m.Name, m.Params, m.Docs)
		}
	}
}

func checkGeneratedDocs(t *testing.T, name string, params []ParamInfo, docs builtinDocs) {
	t.Helper()
	names := map[string]bool{}
	for _, p := range params {
		names[p.Name] = true
	}
	for _, d := range docs {
		if d.Doc == "" {
			continue
		}
		doc := parser.ParseDocBlock(d.Doc)
		tags := strings.Count(d.Doc, " * @")
		read := len(doc.Params)
		if doc.Return.Type != "" {
			read++
			if doc.Return.ParsedType == nil || doc.Return.Description != "" {
				t.Errorf("%s: @return %q isn't read whole", name, doc.Return.Type)
			}
		}
		for _, p := range doc.Params {
			if !names[p.Name] || p.ParsedType == nil || p.Description != "" {
				t.Errorf("%s: @param %q %s isn't read whole or names no parameter", name, p.Type, p.Name)
			}
		}
		if read != tags {
			t.Errorf("%s: read %d of the %d tags in %q", name, read, tags, d.Doc)
		}
	}
}

func TestEmbeddedStubsDontHideGeneratedBuiltinDocs(t *testing.T) {
	profile := DefaultBuiltinProfile()
	profile.Extensions = append(profile.Extensions, "json", "apcu", "igbinary", "ssh2")
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(profile)
	for i := range generatedBuiltinFunctionSignatures {
		sig := &generatedBuiltinFunctionSignatures[i]
		want := sig.Docs.docFor(normalizeBuiltinProfile(profile).PHPVersion)
		if want == "" {
			continue
		}
		if sym := idx.Lookup(sig.Name); sym != nil && sym.DocComment != want {
			t.Errorf("%s is declared by %s, which hides its generated PHPDoc types", sig.Name, sym.URI)
		}
	}
}

func TestEmbeddedStubsDontHideGeneratedBuiltinMethodDocs(t *testing.T) {
	profile := normalizeBuiltinProfile(DefaultBuiltinProfile())
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(profile)
	for _, c := range generatedBuiltinClassLikes {
		for _, m := range c.Members {
			want := m.Docs.docFor(profile.PHPVersion)
			if want == "" || !builtinInVersion(m.Since, m.Removed, profile.PHPVersion) {
				continue
			}
			sym := idx.Lookup(c.Name + "::" + m.Name)
			if sym == nil || sym.DocComment == want || sym.URI != builtinRegistryURI && (sym.DocComment != "" || len(sym.Attributes) > 0) {
				continue
			}
			t.Errorf("%s::%s is declared by %s without PHPDoc or attributes of its own, which hides its generated PHPDoc types", c.Name, m.Name, sym.URI)
		}
	}
}
