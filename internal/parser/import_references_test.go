package parser

import (
	"strings"
	"testing"
)

func TestImportReferences(t *testing.T) {
	cases := []struct {
		name   string
		source string
		used   map[string]bool
	}{
		{"code", "<?php\nuse A\\Post;\nnew Post();\n", map[string]bool{"Post": true}},
		{"own statement only", "<?php\nuse A\\Post;\n", map[string]bool{"Post": false}},
		{"alias", "<?php\nuse A\\Post as Article;\nnew Post();\n", map[string]bool{"Article": false}},
		{"another import statement", "<?php\nuse A\\Models;\nuse A\\Models\\Post;\nnew Post();\n", map[string]bool{"Models": false, "Post": true}},
		{"group import", "<?php\nuse A\\{Post, User};\nnew User();\n", map[string]bool{"Post": false, "User": true}},
		{"line comment", "<?php\nuse A\\Post;\n// Post\n# Post\n", map[string]bool{"Post": false}},
		{"block comment", "<?php\nuse A\\Post;\n/* Post */\n", map[string]bool{"Post": false}},
		{"strings", "<?php\nuse A\\Post;\necho 'Post', \"Post $x\", <<<T\nPost\nT;\n", map[string]bool{"Post": false}},
		{"inline HTML", "<?php\nuse A\\Post;\n?>\n<p>Post</p>\n", map[string]bool{"Post": false}},
		{"variable", "<?php\nuse A\\Post;\n$Post = 1;\n", map[string]bool{"Post": false}},
		{"doc comment", "<?php\nuse A\\Post;\n/** @var array<int, Post>|null */\n", map[string]bool{"Post": true}},
		{"inline var annotation", "<?php\nuse A\\Post;\n/* @var $p Post */\n", map[string]bool{"Post": true}},
		{"inline annotation after a newline", "<?php\nuse A\\Post;\n/*\n @var Post $p */\n", map[string]bool{"Post": true}},
		{"line comment annotation", "<?php\nuse A\\Post;\n// @var Post $p\n", map[string]bool{"Post": false}},
		{"block comment prose", "<?php\nuse A\\Post;\n/* see @Post */\n", map[string]bool{"Post": false}},
		{"doc comment prefix", "<?php\nuse A\\Post;\n/** @var Posts */\n", map[string]bool{"Post": false}},
		{"interpolation", "<?php\nuse A\\Post;\necho \"{$a->make(Post::class)}\";\n", map[string]bool{"Post": true}},
		{"nested interpolation", "<?php\nuse A\\Post;\necho \"{$a->b(\"{$c->d(Post::X)}\")}\";\n", map[string]bool{"Post": true}},
		{"interpolation mentioning an open tag", "<?php\nuse A\\Post;\necho \"{$a->b('<?php', Post::X)}\";\n", map[string]bool{"Post": true}},
		{"trait use", "<?php\nuse A\\HasPosts;\nclass B { use HasPosts; }\n", map[string]bool{"HasPosts": true}},
		{"function and const", "<?php\nuse function A\\fmt;\nuse const A\\MAX;\necho fmt(MAX);\n", map[string]bool{"function fmt": true, "const MAX": true}},
		{"fully qualified", "<?php\nuse A\\User;\nnew \\A\\User();\n", map[string]bool{"User": false}},
		{"qualified uses its first segment", "<?php\nuse A\\Foo;\nuse B\\User;\nnew Foo\\User();\n", map[string]bool{"Foo": true, "User": false}},
		{"namespace relative", "<?php\nuse A\\User;\nnew namespace\\User();\n", map[string]bool{"User": false}},
		{"property", "<?php\nuse A\\User;\necho $x->User;\n", map[string]bool{"User": false}},
		{"nullsafe method", "<?php\nuse A\\User;\n$x?->User();\n", map[string]bool{"User": false}},
		{"member after a comment", "<?php\nuse A\\User;\n$x-> /* c */ User;\n", map[string]bool{"User": false}},
		{"static member", "<?php\nuse A\\Foo;\nuse B\\User;\nFoo::User();\n", map[string]bool{"Foo": true, "User": false}},
		{"interpolated member", "<?php\nuse A\\User;\necho \"{$x->User}\";\n", map[string]bool{"User": false}},
		{"variable variable interpolation", "<?php\nuse A\\Names;\necho \"${Names::FIRST}\";\n", map[string]bool{"Names": true}},
		{"namespace declaration", "<?php\nnamespace App\\Http;\nuse B\\App;\n", map[string]bool{"App": false}},
		{"declared names", "<?php\nuse A\\User;\nuse A\\Post;\nclass Post { function User() {} }\n", map[string]bool{"User": false, "Post": false}},
		{"function import called", "<?php\nuse function A\\fmt;\nfmt /* c */ ('x');\n", map[string]bool{"function fmt": true}},
		{"function import not called", "<?php\nuse function A\\fmt;\n$x->fmt();\nfmt::X;\nnew fmt();\necho fmt, fmt\\x();\n", map[string]bool{"function fmt": false}},
		{"const import read", "<?php\nuse const A\\MAX;\necho MAX;\n", map[string]bool{"const MAX": true}},
		{"const import not read", "<?php\nuse const A\\MAX;\nMAX();\nMAX::X;\nnew MAX;\n$x->MAX;\n", map[string]bool{"const MAX": false}},
		{"class import of a called name", "<?php\nuse A\\Route;\n#[Route('/')]\nfunction f() {}\n", map[string]bool{"Route": true}},
		{"class and function import of one name", "<?php\nuse A\\fmt;\nuse function B\\fmt;\nnew fmt();\n", map[string]bool{"fmt": true, "function fmt": false}},
		{"doc comment counts for every kind", "<?php\nuse function A\\fmt;\nuse const A\\MAX;\n/** @see fmt() MAX */\n", map[string]bool{"function fmt": true, "const MAX": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refs := New().Parse(tc.source).ImportReferences()
			for imported, want := range tc.used {
				kind, alias, ok := strings.Cut(imported, " ")
				if !ok {
					kind, alias = "class", imported
				}
				if got := refs.Used(UseNode{Alias: alias, Kind: kind}); got != want {
					t.Errorf("Used(%s) = %v, want %v", imported, got, want)
				}
			}
		})
	}
}

func TestImportReferencesRejectsUnknownKind(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Used with an unknown import kind did not panic")
		}
	}()
	New().Parse("<?php\n").ImportReferences().Used(UseNode{Alias: "Post", Kind: "type"})
}
