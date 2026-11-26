package analyzer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestDefinitionOfNamesInACallSplitOverLines(t *testing.T) {
	t.Run("Laravel view", func(t *testing.T) {
		root := laravelViewTestdataPath()
		idx := symbols.NewIndex()
		a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, root, "laravel"))
		a.SetViewResolver(frameworklaravel.NewViews(root))
		src := "<?php\nfunction demo() {\n    return view(\n        'pages::auth.login',\n        []\n    );\n}\n"
		loc := a.FindDefinition("file:///app/demo.php", src, posOf(t, src, "auth.login"))
		if loc == nil || !strings.Contains(loc.URI, "/resources/views/pages/auth/login.blade.php") {
			t.Fatalf("got %+v", loc)
		}
		if loc := a.FindDefinition("file:///app/demo.php", "<?php\n// view('pages::auth.login');\n", posOf(t, "<?php\n// view('pages::auth.login');\n", "auth.login")); loc != nil {
			t.Errorf("a view name in a comment: got %+v", loc)
		}
	})

	t.Run("Laravel translation key", func(t *testing.T) {
		a := setupTranslationAnalyzer(t)
		src := "<?php\necho __(\n    'messages.welcome'\n);\n"
		loc := a.FindDefinition("file:///test.php", src, posOf(t, src, "welcome"))
		if loc == nil || !strings.Contains(loc.URI, "/lang/es/messages.php") {
			t.Fatalf("got %+v", loc)
		}
	})

	t.Run("Symfony route", func(t *testing.T) {
		a, controller := setupSymfonyRouteAnalyzer(t)
		const uri = "file:///src/Controller/ProductController.php"
		want := a.FindDefinition(uri, controller, posOf(t, controller, "product_show'"))
		if want == nil {
			t.Fatal("the route resolves on one line")
		}
		src := "<?php\nclass X extends \\Symfony\\Bundle\\FrameworkBundle\\Controller\\AbstractController {\n    public function f() {\n        return $this->generateUrl(\n            'product_show',\n            ['id' => 1]\n        );\n    }\n}\n"
		loc := a.FindDefinition("file:///src/x.php", src, posOf(t, src, "duct_show"))
		if loc == nil || *loc != *want {
			t.Fatalf("got %+v, want %+v", loc, want)
		}
	})

	t.Run("Symfony translation key", func(t *testing.T) {
		root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "symfony"))
		if err != nil {
			t.Fatal(err)
		}
		idx := symbols.NewIndex()
		a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, root, "symfony"))
		a.SetSymfonyTranslationResolver(frameworksymfony.NewTranslationResolver(root))
		src := "<?php\n$translator->trans(\n    'nav.home',\n    [],\n    'messages'\n);\n"
		loc := a.FindDefinition("file:///x.php", src, posOf(t, src, "home"))
		if loc == nil || !strings.HasSuffix(loc.URI, "messages.en.yaml") {
			t.Fatalf("got %+v", loc)
		}
	})
}

func TestDefinitionOfAContainerArgumentReadsTheCall(t *testing.T) {
	a := setupLaravelAnalyzer(t)
	for _, src := range []string{
		"<?php\napp(\n    'request'\n);\n",
		"<?php\napp(/* ) */ 'request');\n",
		"<?php\n$this->app->make(\n    // the request\n    'request',\n    [],\n);\n",
	} {
		loc := a.FindDefinition("file:///test.php", src, posOf(t, src, "quest'"))
		if loc == nil || !strings.HasSuffix(loc.URI, "/Http/Request.php") {
			t.Errorf("%q: got %+v", src, loc)
		}
		if locs := a.FindTypeDefinition("file:///test.php", src, posOf(t, src, "quest'")); len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "/Http/Request.php") {
			t.Errorf("%q: type definition %+v", src, locs)
		}
	}
	for _, src := range []string{
		"<?php\napp('db', 'request');\n",
		"<?php\n// app('request');\n",
		"<?php\nfoo('app(', 'request');\n",
	} {
		if loc := a.FindDefinition("file:///test.php", src, posOf(t, src, "quest'")); loc != nil && strings.HasSuffix(loc.URI, "/Http/Request.php") {
			t.Errorf("%q is not a container argument: got %+v", src, loc)
		}
	}
	src := "<?php\napp($request);\n"
	if loc := a.FindDefinition("file:///test.php", src, posOf(t, src, "quest)")); loc != nil && strings.HasSuffix(loc.URI, "/Http/Request.php") {
		t.Errorf("a variable argument: got %+v", loc)
	}
}

func TestDefinitionOfALaravelRouteNameReadsTheCall(t *testing.T) {
	a := setupLaravelRouteAnalyzer(t)
	for _, src := range []string{
		"<?php\nreturn route(\n    'dashboard'\n);\n",
		"<?php\nreturn route(/* ) */ 'dashboard');\n",
		"<?php\nreturn to_route(\n    'dashboard', // home\n    [],\n);\n",
	} {
		loc := a.FindDefinition("file:///test.php", src, posOf(t, src, "board"))
		if loc == nil || !strings.Contains(loc.URI, "/routes/web.php") {
			t.Errorf("%q: got %+v", src, loc)
		}
	}
	for _, src := range []string{
		"<?php\n// route('dashboard');\n",
		"<?php\necho \"route(\" . e('dashboard');\n",
		"<?php\nreturn route('home', ['dashboard']);\n",
	} {
		if loc := a.FindDefinition("file:///test.php", src, posOf(t, src, "board")); loc != nil {
			t.Errorf("%q is not a route name: got %+v", src, loc)
		}
	}
}
