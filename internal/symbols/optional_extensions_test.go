package symbols

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/stubs"
)

func TestOptionalExtensionSymbols(t *testing.T) {
	cases := []struct {
		extension string
		class     string
		method    string
		params    []ParamInfo
		ret       string
	}{
		{"imagick", "Imagick", "readImage", []ParamInfo{{Name: "$filename", Type: "string"}}, "bool"},
		{"redis", "Redis", "get", []ParamInfo{{Name: "$key", Type: "string"}}, "mixed"},
		{"memcached", "Memcached", "set", []ParamInfo{{Name: "$key", Type: "string"}, {Name: "$value", Type: "mixed"}, {Name: "$expiration", Type: "int", DefaultValue: "0"}}, "bool"},
	}
	for _, tc := range cases {
		t.Run(tc.extension, func(t *testing.T) {
			idx := NewIndex()
			idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "7.4", Extensions: []string{tc.extension}})
			sym := idx.Lookup(tc.class + "::" + tc.method)
			if sym == nil || sym.Kind != KindMethod || sym.Source != SourceBuiltin {
				t.Fatalf("missing builtin method: %+v", sym)
			}
			if sym.ReturnType != tc.ret || len(sym.Params) != len(tc.params) {
				t.Fatalf("signature = (%+v): %s", sym.Params, sym.ReturnType)
			}
			for i, want := range tc.params {
				if sym.Params[i] != want {
					t.Errorf("param %d = %+v, want %+v", i, sym.Params[i], want)
				}
			}
		})
	}
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "7.4", Extensions: []string{"ssh2", "apcu", "igbinary"}})
	for _, name := range []string{"ssh2_connect", "apcu_fetch", "igbinary_serialize", "igbinary_unserialize"} {
		if sym := idx.Lookup(name); sym == nil || sym.Kind != KindFunction || sym.Source != SourceBuiltin {
			t.Errorf("missing builtin function %s: %+v", name, sym)
		}
	}
	connect := idx.Lookup("ssh2_connect")
	if connect == nil || len(connect.Params) != 4 || connect.Params[1].DefaultValue != "22" || connect.Params[2].Type != "?array" {
		t.Fatalf("ssh2_connect parameters: %+v", connect)
	}
	doc := parser.ParseDocBlock(connect.DocComment)
	if doc == nil || len(doc.Tags["return"]) != 1 || doc.Tags["return"][0] != "resource|false" {
		t.Fatalf("ssh2_connect resource return type: %+v", doc)
	}
	for _, name := range []string{"SSH2_FINGERPRINT_SHA1", "APC_ITER_ALL"} {
		if sym := idx.Lookup(name); sym == nil || sym.Kind != KindConstant || sym.Source != SourceBuiltin {
			t.Errorf("missing builtin constant %s: %+v", name, sym)
		}
	}
}

func TestOptionalExtensionAvailabilityAndIsolation(t *testing.T) {
	// Walk declarations, not a hand-picked subset, so additions cannot silently
	// miss availability metadata or leak into the unselected index.
	entries, err := stubs.BuiltinPHPForProfile(stubs.Profile{PHPVersion: "7.4", Extensions: []string{"imagick", "ssh2", "redis", "memcached", "apcu", "igbinary"}})
	if err != nil {
		t.Fatal(err)
	}
	without := NewIndex()
	without.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "7.4"})
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Path, "php/extensions/") {
			continue
		}
		ext := strings.Split(entry.Path, "/")[2]
		file := parser.ParseFile(entry.Content)
		names := make([]string, 0, len(file.Classes)+len(file.Functions)+len(file.Constants))
		for _, class := range file.Classes {
			names = append(names, class.Name)
		}
		for _, fn := range file.Functions {
			names = append(names, fn.Name)
		}
		for _, constant := range file.Constants {
			names = append(names, constant.Name)
		}
		for _, name := range names {
			availability, ok := LookupAvailability(name)
			if !ok || availability.Extension != ext {
				t.Errorf("%s availability = %+v, %v; want ext-%s", name, availability, ok, ext)
			}
			if sym := without.Lookup(name); sym != nil {
				t.Errorf("unselected extension symbol was indexed: %s", name)
			}
		}
	}
	if got, ok := LookupAvailability("Imagick"); !ok || got.Extension != "imagick" {
		t.Fatalf("Imagick must require ext-imagick: %+v, %v", got, ok)
	}
}

func TestOptionalExtensionAPICoverage(t *testing.T) {
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "7.4", Extensions: []string{"imagick", "ssh2", "redis", "memcached", "apcu", "igbinary"}})
	// Counts taken from the extension-owned public declarations. This also
	// catches parser recovery that silently drops part of a large API file.
	for _, tc := range []struct {
		class     string
		methods   int
		constants int
	}{
		{"Imagick", 423, 661},
		{"ImagickDraw", 135, 0},
		{"ImagickPixel", 21, 0},
		{"ImagickPixelIterator", 21, 0},
		{"ImagickKernel", 7, 0},
		{"Redis", 254, 45},
		{"RedisArray", 31, 0},
		{"RedisCluster", 222, 5},
		{"RedisSentinel", 12, 0},
		{"Memcached", 59, 150},
		{"APCUIterator", 9, 0},
	} {
		t.Run(tc.class, func(t *testing.T) {
			class := idx.Lookup(tc.class)
			if class == nil {
				t.Fatal("missing class")
			}
			var methods, constants int
			for _, member := range class.Children {
				switch member.Kind {
				case KindMethod:
					methods++
				case KindConstant:
					constants++
				}
			}
			if methods != tc.methods || constants != tc.constants {
				t.Errorf("got %d methods and %d constants, want %d and %d", methods, constants, tc.methods, tc.constants)
			}
		})
	}
	for _, name := range []string{"ImagickException", "ImagickDrawException", "ImagickPixelException", "ImagickPixelIteratorException", "ImagickKernelException", "RedisException", "RedisClusterException", "MemcachedException"} {
		if sym := idx.Lookup(name); sym == nil || sym.Kind != KindClass || sym.Extends == "" {
			t.Errorf("missing exception class/inheritance: %s", name)
		}
	}
	if sym := idx.Lookup("Imagick::COMPOSITE_OVER"); sym == nil || sym.Type != "int" {
		t.Errorf("missing constant type: %+v", sym)
	}
	if sym := idx.Lookup("Redis::scan"); sym == nil || len(sym.Params) < 1 || !sym.Params[0].IsReference {
		t.Errorf("scan iterator must be passed by reference: %+v", sym)
	}
	for _, tc := range []struct {
		extension string
		functions int
		constants int
	}{
		{"ssh2", 41, 29},
		{"apcu", 14, 15},
		{"igbinary", 2, 0},
	} {
		var functions, constants int
		for _, sym := range idx.GetFileSymbols("builtin://php/extensions/" + tc.extension + "/php-7.4.php") {
			if sym.Kind == KindFunction {
				functions++
			} else if sym.Kind == KindConstant && sym.ParentFQN == "" {
				constants++
			}
		}
		if functions != tc.functions || constants != tc.constants {
			t.Errorf("%s: got %d functions and %d global constants, want %d and %d", tc.extension, functions, constants, tc.functions, tc.constants)
		}
	}
}

func BenchmarkRegisterOptionalExtensionBuiltins(b *testing.B) {
	for _, tc := range []struct {
		name       string
		extensions []string
	}{
		{"core", nil},
		{"optional", []string{"imagick", "ssh2", "redis", "memcached", "apcu", "igbinary"}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				idx := NewIndex()
				idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "7.4", Extensions: tc.extensions})
			}
		})
	}
}
