package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// symfonyFixture starts a server on testdata/symfony, waits for indexing and
// the console command scan, and opens src as an unsaved controller.
func symfonyFixture(t *testing.T, src string) (*lspHarness, string, string) {
	t.Helper()
	root, err := filepath.Abs("../../testdata/symfony")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "vendor", "symfony", "framework-bundle")); err != nil {
		t.Skip("testdata/symfony/vendor is missing")
	}
	h := newHarness(t)
	h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": "file://" + root, "capabilities": map[string]interface{}{}}))
	h.notify("initialized", map[string]interface{}{})
	deadline := time.Now().Add(30 * time.Second)
	for !h.server.index.Ready() || h.server.workspace.SymfonyCommands.Find("app:sync-products") == nil {
		if time.Now().After(deadline) {
			h.close()
			t.Fatal("indexing or the console command scan never finished")
		}
		time.Sleep(20 * time.Millisecond)
	}
	uri := "file://" + filepath.Join(root, "src/Controller/ProbeController.php")
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "php", "version": 1, "text": src},
	})
	return h, uri, root
}

func positionOf(t *testing.T, src, anchor string, offset int) map[string]interface{} {
	t.Helper()
	i := strings.Index(src, anchor)
	if i < 0 {
		t.Fatalf("anchor %q not found", anchor)
	}
	i += offset
	return map[string]interface{}{
		"line":      strings.Count(src[:i], "\n"),
		"character": i - strings.LastIndex(src[:i], "\n") - 1,
	}
}

func definitionURI(t *testing.T, h *lspHarness, uri string, pos map[string]interface{}) string {
	t.Helper()
	raw, _ := json.Marshal(h.readResponse(h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri}, "position": pos,
	}))["result"])
	var loc struct{ URI string }
	_ = json.Unmarshal(raw, &loc)
	return loc.URI
}

func TestSymfonyTranslationKeysThroughTheServer(t *testing.T) {
	src := `<?php
namespace App\Controller;

use Symfony\Contracts\Translation\TranslatorInterface;

class ProbeController
{
    public function show(TranslatorInterface $translator)
    {
        $translator->trans('nav.home');
        $translator->trans('welcome_message');
        $translator->trans('logout', [], 'security');
        $translator->trans('');
    }
}
`
	h, uri, root := symfonyFixture(t, src)
	defer h.close()

	got := responseText(h.readResponse(h.send("textDocument/completion", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri}, "position": positionOf(t, src, "trans('')", 7),
	})))
	for _, key := range []string{"nav.home", "user.profile.title", "welcome_message", "login.title"} {
		if !strings.Contains(strings.ToLower(got), `"label":"`+strings.ToLower(key)+`"`) {
			t.Errorf("trans(' completion is missing %s: %.400s", key, got)
		}
	}

	for anchor, file := range map[string]string{
		"nav.home')":        "messages.en.yaml",
		"welcome_message')": "messages.en.xlf",
		"logout'":           "security.en.php",
	} {
		if got := definitionURI(t, h, uri, positionOf(t, src, anchor, 2)); got != "file://"+filepath.Join(root, "translations", file) {
			t.Errorf("definition of %s: got %q, want %s", anchor, got, file)
		}
	}
}

func TestSymfonyConsoleCommandsThroughTheServer(t *testing.T) {
	src := `<?php
namespace App\Controller;

use Psr\Container\ContainerInterface;
use Symfony\Bundle\FrameworkBundle\Console\Application;

class ProbeController
{
    public function run(Application $application, ContainerInterface $container)
    {
        $application->find('app:sync-products');
        $application->find('');
        $container->get('');
        $container->get('app:sync-products');
    }
}
`
	h, uri, root := symfonyFixture(t, src)
	defer h.close()

	complete := func(anchor string) string {
		return responseText(h.readResponse(h.send("textDocument/completion", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri}, "position": positionOf(t, src, anchor, len(anchor)-2),
		})))
	}
	if got := complete("$application->find('')"); !strings.Contains(got, `"label":"app:sync-products"`) || !strings.Contains(got, `"label":"app:import-legacy"`) {
		t.Errorf("$application->find(' completion lacks the project's commands: %.400s", got)
	}
	if got := complete("$container->get('')"); strings.Contains(got, `"label":"app:sync-products"`) || strings.Contains(got, `"label":"nav.home"`) {
		t.Errorf("$container->get(' offered console commands or translation keys: %.400s", got)
	}

	want := "file://" + filepath.Join(root, "src/Command/SyncProductsCommand.php")
	if got := definitionURI(t, h, uri, positionOf(t, src, "app:sync-products');\n        $application", 4)); got != want {
		t.Errorf("definition of the command name: got %q, want %q", got, want)
	}
	if got := definitionURI(t, h, uri, positionOf(t, src, "app:sync-products');\n    }", 4)); got == want {
		t.Error("$container->get('app:sync-products') went to the console command")
	}

	// A command added while the server runs is found without a restart.
	cmdURI := "file://" + filepath.Join(root, "src/Command/ProbeCommand.php")
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": cmdURI, "languageId": "php", "version": 1, "text": `<?php
namespace App\Command;
use Symfony\Component\Console\Attribute\AsCommand;
use Symfony\Component\Console\Command\Command;
#[AsCommand(name: 'app:probe', description: 'Added while running')]
class ProbeCommand extends Command {}
`},
	})
	if got := complete("$application->find('')"); !strings.Contains(got, `"label":"app:probe"`) {
		t.Errorf("a newly opened command is missing: %.400s", got)
	}
}
