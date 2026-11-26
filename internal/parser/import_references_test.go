package parser

import "testing"

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
		{"function and const", "<?php\nuse function A\\fmt;\nuse const A\\MAX;\necho fmt(MAX);\n", map[string]bool{"fmt": true, "MAX": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refs := New().Parse(tc.source).ImportReferences()
			for alias, want := range tc.used {
				if got := refs.Used(UseNode{Alias: alias}); got != want {
					t.Errorf("Used(%s) = %v, want %v", alias, got, want)
				}
			}
		})
	}
}
