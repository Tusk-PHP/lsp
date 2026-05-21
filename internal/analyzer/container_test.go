package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func laravelTestdataPath() string {
	return filepath.Join("..", "..", "testdata", "laravel")
}

func symfonyTestdataPath() string {
	return filepath.Join("..", "..", "testdata", "symfony")
}

func readLaravelFile(t *testing.T, relPath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(laravelTestdataPath(), relPath))
	if err != nil {
		t.Fatalf("failed to read %s: %v", relPath, err)
	}
	return string(content)
}

func indexPHPDir(t *testing.T, idx *symbols.Index, root, dir string, src symbols.SymbolSource) {
	t.Helper()
	absDir := filepath.Join(root, dir)
	filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		idx.IndexFileWithSource("file:///"+rel, string(data), src)
		return nil
	})
}

func setupLaravelAnalyzer(t *testing.T) *Analyzer {
	t.Helper()
	root := laravelTestdataPath()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	indexPHPDir(t, idx, root, "app", symbols.SourceProject)
	indexPHPDir(t, idx, root, "vendor/laravel/framework/src", symbols.SourceVendor)

	ca := container.NewContainerAnalyzer(idx, root, "laravel")
	ca.Analyze()

	return NewAnalyzer(idx, ca)
}

func setupSymfonyAnalyzer(t *testing.T) *Analyzer {
	t.Helper()
	root := symfonyTestdataPath()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	indexPHPDir(t, idx, root, "src", symbols.SourceProject)
	indexPHPDir(t, idx, root, "vendor/symfony/framework-bundle", symbols.SourceVendor)
	indexPHPDir(t, idx, root, "vendor/symfony/http-foundation", symbols.SourceVendor)
	indexPHPDir(t, idx, root, "vendor/symfony/dependency-injection", symbols.SourceVendor)
	indexPHPDir(t, idx, root, "vendor/symfony/http-kernel", symbols.SourceVendor)
	indexPHPDir(t, idx, root, "vendor/psr/container", symbols.SourceVendor)

	ca := container.NewContainerAnalyzer(idx, root, "symfony")
	ca.Analyze()

	return NewAnalyzer(idx, ca)
}

func TestDefinitionAppStringBinding(t *testing.T) {
	a := setupLaravelAnalyzer(t)

	// Clicking on 'request' inside app('request') should go to Illuminate\Http\Request
	source := `<?php
namespace App\Http\Controllers;

class TestController {
    public function index() {
        app('request');
    }
}
`
	pos := protocol.Position{Line: 5, Character: 14} // on 'request'
	loc := a.FindDefinition("file:///test.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition for app('request')")
	}
	if !strings.Contains(loc.URI, "Request.php") {
		t.Errorf("expected URI containing Request.php, got %s", loc.URI)
	}
}

func TestDefinitionAppClassBinding(t *testing.T) {
	a := setupLaravelAnalyzer(t)

	// app(User::class) should go to User class
	source := `<?php
namespace App\Http\Controllers;

use App\Models\User;

class TestController {
    public function index() {
        app(User::class);
    }
}
`
	pos := protocol.Position{Line: 7, Character: 13} // on 'User'
	loc := a.FindDefinition("file:///test.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition for app(User::class)")
	}
	if !strings.Contains(loc.URI, "User.php") {
		t.Errorf("expected URI containing User.php, got %s", loc.URI)
	}
}

func TestDefinitionAppChainedMethod(t *testing.T) {
	a := setupLaravelAnalyzer(t)

	// app('request')->url() — clicking on 'url' should go to Request::url
	source := `<?php
namespace App\Http\Controllers;

class TestController {
    public function index() {
        app('request')->url();
    }
}
`
	pos := protocol.Position{Line: 5, Character: 26} // on 'url'
	loc := a.FindDefinition("file:///test.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition for url() on app('request')->url()")
	}
	if !strings.Contains(loc.URI, "Request.php") {
		t.Errorf("expected URI containing Request.php, got %s", loc.URI)
	}
}

func TestDefinitionResolveHelper(t *testing.T) {
	a := setupLaravelAnalyzer(t)

	// resolve('request') should go to Request class
	source := `<?php
namespace App\Http\Controllers;

class TestController {
    public function index() {
        resolve('request');
    }
}
`
	pos := protocol.Position{Line: 5, Character: 17} // on 'request'
	loc := a.FindDefinition("file:///test.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition for resolve('request')")
	}
	if !strings.Contains(loc.URI, "Request.php") {
		t.Errorf("expected URI containing Request.php, got %s", loc.URI)
	}
}

func TestDefinitionSymfonyServiceIDGoesToServiceConfig(t *testing.T) {
	a := setupSymfonyAnalyzer(t)

	source := `<?php
namespace App\Controller;

use Symfony\Component\DependencyInjection\ContainerInterface;

class TestController {
    public function __construct(private ContainerInterface $container) {}

    public function index(): void {
        $this->container->get('app.notifier');
    }
}
`
	pos := protocol.Position{Line: 9, Character: 31}
	loc := a.FindDefinition("file:///test.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition for Symfony service ID")
	}
	if !strings.Contains(loc.URI, "services.yaml") {
		t.Fatalf("expected services.yaml definition, got %s", loc.URI)
	}
	if loc.Range.Start.Line != 5 {
		t.Fatalf("expected alias definition on line 5, got %d", loc.Range.Start.Line)
	}
}

func TestTypeDefinitionSymfonyServiceIDGoesToConcreteClass(t *testing.T) {
	a := setupSymfonyAnalyzer(t)

	source := `<?php
namespace App\Controller;

use Symfony\Component\DependencyInjection\ContainerInterface;

class TestController {
    public function __construct(private ContainerInterface $container) {}

    public function index(): void {
        $this->container->get('app.payment_processor');
    }
}
`
	pos := protocol.Position{Line: 9, Character: 31}
	locs := a.FindTypeDefinition("file:///test.php", source, pos)
	if len(locs) == 0 {
		t.Fatal("expected type definition for Symfony service ID")
	}
	if !strings.Contains(locs[0].URI, "PaymentProcessor.php") {
		t.Fatalf("expected PaymentProcessor.php type definition, got %s", locs[0].URI)
	}
}
