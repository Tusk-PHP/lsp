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
		{"doc comment references to a function and a constant", "<?php\nuse function A\\fmt;\nuse const A\\MAX;\n/** @see fmt() */\n/** @see MAX */\n", map[string]bool{"function fmt": true, "const MAX": true}},
		{"doc comment prose after a reference", "<?php\nuse function A\\fmt;\nuse const A\\MAX;\n/** @see fmt() MAX */\n", map[string]bool{"function fmt": true, "const MAX": false}},
		{"class import in another case", "<?php\nuse A\\Post;\nuse A\\User;\nuse A\\Tag;\nnew post();\nUSER::find();\necho tag\\X::class;\n", map[string]bool{"Post": true, "User": true, "Tag": true}},
		{"lower-case class alias", "<?php\nuse A\\Post as post;\nnew POST();\n", map[string]bool{"post": true}},
		{"non-ASCII letters keep their case", "<?php\nuse A\\Éclair;\nnew éclair();\n", map[string]bool{"Éclair": false}},
		{"function import in another case", "<?php\nuse function A\\fmt;\nFMT('x');\n", map[string]bool{"function fmt": true}},
		{"const import in another case", "<?php\nuse const A\\MAX;\necho max, Max;\n", map[string]bool{"const MAX": false}},
		{"class and const import folding to one name", "<?php\nuse A\\foo;\nuse const B\\FOO;\necho Foo;\n", map[string]bool{"foo": true, "const FOO": false}},
		{"doc comment words keep the import's spelling", "<?php\nuse A\\Post;\nuse function A\\fmt;\nuse const A\\MAX;\n/** Loads the post, @see FMT() max */\n", map[string]bool{"Post": false, "function fmt": false, "const MAX": false}},
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

func TestImportReferencesInDocComments(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		used map[string]bool
	}{
		{"param", "/** @param Post $p */", map[string]bool{"Post": true}},
		{"var", "/** @var Post */", map[string]bool{"Post": true}},
		{"return", "/** @return Post the post */", map[string]bool{"Post": true}},
		{"throws", "/** @throws Post when missing */", map[string]bool{"Post": true}},
		{"properties", "/**\n * @property Post $a\n * @property-read User $b\n * @property-write Tag $c\n */", map[string]bool{"Post": true, "User": true, "Tag": true}},
		{"untyped property", "/** @property $a */", map[string]bool{"Post": false}},
		{"mixin", "/** @mixin Post */", map[string]bool{"Post": true}},
		{"template bound and default", "/**\n * @template T of Post\n * @template-covariant U = User\n */", map[string]bool{"Post": true, "User": true}},
		{"extends implements use", "/**\n * @template-extends Post<User>\n * @implements Tag<int>\n * @template-use Comment<int>\n */", map[string]bool{"Post": true, "User": true, "Tag": true, "Comment": true}},
		{"require extends and implements", "/**\n * @phpstan-require-extends Post\n * @phpstan-require-implements User\n */", map[string]bool{"Post": true, "User": true}},
		{"phpstan and psalm variants", "/**\n * @phpstan-param Post $p\n * @psalm-return User\n * @phpstan-var Tag\n * @phpstan-self-out Comment\n */", map[string]bool{"Post": true, "User": true, "Tag": true, "Comment": true}},
		{"param out and closure this", "/**\n * @param-out Post $p\n * @param-closure-this User $c\n */", map[string]bool{"Post": true, "User": true}},
		{"assert", "/** @phpstan-assert-if-true !Post $p */", map[string]bool{"Post": true}},
		{"type alias", "/** @phpstan-type Row array{post: Post} */", map[string]bool{"Post": true}},
		{"imported type alias", "/** @phpstan-import-type Row from Post as PostRow */", map[string]bool{"Post": true, "PostRow": false}},
		{"global", "/**\n * @global WP_Post $post The post.\n * @global $wpdb\n */", map[string]bool{"WP_Post": true, "The": false}},
		{"call-site variance", "/** @param Repository<covariant Post, contravariant User> $r */", map[string]bool{"Repository": true, "Post": true, "User": true}},
		{"sealed, inheritors and the like", "/**\n * @phpstan-sealed Post|User\n * @psalm-inheritors Tag\n * @psalm-if-this-is Comment\n * @psalm-scope-this Author\n * @psalm-yield Draft\n */", map[string]bool{"Post": true, "User": true, "Tag": true, "Comment": true, "Author": true, "Draft": true}},
		{"expected exception", "/** @expectedException Post when missing */", map[string]bool{"Post": true}},
		{"WordPress hash type", "/** @type Post $post */", map[string]bool{"Post": true}},
		{"generics", "/** @param Collection<int, Post> $p */", map[string]bool{"Collection": true, "Post": true}},
		{"union, intersection and nullable", "/** @return ?Post|(User&Tag) */", map[string]bool{"Post": true, "User": true, "Tag": true}},
		{"array of", "/** @var Post[] */", map[string]bool{"Post": true}},
		{"class string", "/** @param class-string<Post> $c */", map[string]bool{"Post": true}},
		{"callable signature", "/** @param callable(Post $p, User...): Tag $cb */", map[string]bool{"Post": true, "User": true, "Tag": true}},
		{"generic closure signature", "/** @param string|\\Closure<T of Post, U>(User|Tag, T):(object|null)|null $expr */", map[string]bool{"Post": true, "User": true, "Tag": true}},
		{"method parameter default", "/** @method $this add(string $type = Post::class, $n = User::LIMIT) */", map[string]bool{"Post": true, "User": true}},
		{"method parameters with commas", "/** @method void f(array{a: Post, b: int} $x, Closure(User, int): void $cb, array $d = [Tag::A, 2], string $s = 'Comment,x') */", map[string]bool{"Post": true, "User": true, "Tag": true, "Comment": false}},
		{"closure signature", "/** @param Closure(Post): void $cb */", map[string]bool{"Closure": true, "Post": true}},
		{"array shape", "/** @return array{post: Post, tags: list<Tag>, 'x'?: User} */", map[string]bool{"Post": true, "Tag": true, "User": true}},
		{"array shape over lines", "/**\n * @return array{\n *     post: Post,\n *     user: User,\n * }\n */", map[string]bool{"Post": true, "User": true}},
		{"conditional return", "/** @return ($x is true ? Post : User) */", map[string]bool{"Post": true, "User": true}},
		{"offset access", "/** @return Post[User] */", map[string]bool{"Post": true, "User": true}},
		{"class constant type", "/** @param Post::STATUS_* $s */", map[string]bool{"Post": true}},
		{"literal type", "/** @param 'Post'|1 $s */", map[string]bool{"Post": false}},
		{"method", "/** @method Post find(User $u, int $n = 1, ...$rest) Finds a Tag */", map[string]bool{"Post": true, "User": true, "Tag": false}},
		{"method without parentheses", "/** @method Post getPost */", map[string]bool{"Post": true}},
		{"static method", "/** @method static Collection<int, Post> all() */", map[string]bool{"Collection": true, "Post": true}},
		{"see", "/** @see Post */", map[string]bool{"Post": true}},
		{"see method", "/** @see Post::find() for details */", map[string]bool{"Post": true}},
		{"see URL", "/** @see https://example.com/Post */", map[string]bool{"https": false}},
		{"link", "/** @link User */", map[string]bool{"User": true}},
		{"covers and uses", "/**\n * @covers Post::find\n * @uses User\n * @coversDefaultClass Tag\n */", map[string]bool{"Post": true, "User": true, "Tag": true}},
		{"inline see and link", "/**\n * Loads a {@see Post} through {@link User::find() the finder}.\n * {@inheritDoc}\n */", map[string]bool{"Post": true, "User": true}},
		{"unterminated inline see", "/** Loads a {@see Post */", map[string]bool{"Post": false}},
		{"inline var with the name first", "/* @var $p Post */", map[string]bool{"Post": true}},
		{"var with only a name", "/* @var $p */", map[string]bool{"Post": false}},
		{"fully qualified", "/** @var \\A\\Post */", map[string]bool{"Post": false}},
		{"qualified uses its first segment", "/** @var Models\\Post */", map[string]bool{"Models": true, "Post": false}},
		{"another case", "/** @param post $p @see USER */\n/** @see USER */", map[string]bool{"Post": true, "User": true}},
		{"type keyword prefix", "/** @param non-empty-string $non */", map[string]bool{"non": false}},
		{"annotation description lines", "/**\n * @SuppressWarnings(PHPMD.CouplingBetweenObjects) - uses constants:\n *      BasePrice::PRICE_CODE\n * @since 1.0\n * User text\n */", map[string]bool{"BasePrice": true, "User": false}},
		{"annotation", "/**\n * @ORM\\Column(type=\"string\")\n * @Route(\"/posts\", name=\"Post\")\n * @Assert\\All({@Constraint(User::class)})\n */", map[string]bool{"ORM": true, "Route": true, "Post": false, "Assert": true, "Constraint": true, "User": true}},
		{"lower-case unknown tag", "/** @group Post */", map[string]bool{"Post": false}},
		{"prose", "/**\n * Ajax grid action.\n * Executes the Post command.\n */", map[string]bool{"Action": false, "Command": false, "Post": false}},
		{"names and descriptions", "/**\n * @param OrderPaymentInterface $payment the Post to load\n * @return void Returns no User\n * @deprecated use Tag instead\n * @todo Comment\n */", map[string]bool{"Payment": false, "Post": false, "User": false, "Tag": false, "Comment": false, "OrderPaymentInterface": true}},
		{"description lines after a tag", "/**\n * @param Post $p\n *     loaded from the User table\n */", map[string]bool{"Post": true, "User": false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var src strings.Builder
			src.WriteString("<?php\n")
			for alias := range tc.used {
				src.WriteString("use A\\" + alias + ";\n")
			}
			src.WriteString(tc.doc + "\n")
			refs := New().Parse(src.String()).ImportReferences()
			for alias, want := range tc.used {
				if got := refs.Used(UseNode{Alias: alias, Kind: "class"}); got != want {
					t.Errorf("Used(%s) = %v, want %v", alias, got, want)
				}
			}
		})
	}
}
