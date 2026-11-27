package lsp

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func waitForBinding(t *testing.T, h *lspHarness, service string, bound bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for (h.server.container.LookupBinding(service) != nil) != bound {
		if time.Now().After(deadline) {
			t.Fatalf("%s bound = %v, want %v", service, !bound, bound)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSavingSymfonyServicesYAMLUpdatesTheContainer(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":        `{"require": {"symfony/framework-bundle": "^7.0"}}`,
		"config/services.yaml": "services:\n    app.mailer:\n        class: App\\Mailer\n",
	}, map[string]interface{}{}, nil)
	<-h.server.indexed
	waitForBinding(t, h, "app.mailer", true)

	writeProjectFiles(t, root, map[string]string{"config/services.yaml": "services:\n    app.notifier:\n        class: App\\Notifier\n"})
	uri := symbols.PathToURI(filepath.Join(root, "config", "services.yaml"))
	h.notify("textDocument/didSave", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})

	waitForBinding(t, h, "app.notifier", true)
	waitForBinding(t, h, "app.mailer", false)
}

func TestSymfonyServicesYAMLChangedOutsideTheEditorUpdatesTheContainer(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{
		"composer.json":        `{"require": {"symfony/framework-bundle": "^7.0"}}`,
		"config/services.yaml": "services:\n    app.mailer:\n        class: App\\Mailer\n",
	}, map[string]interface{}{}, nil)
	<-h.server.indexed
	waitForBinding(t, h, "app.mailer", true)

	writeProjectFiles(t, root, map[string]string{"config/services.yaml": "services:\n    app.notifier:\n        class: App\\Notifier\n"})
	notifyWatched(h, root, "config/services.yaml")

	waitForBinding(t, h, "app.notifier", true)
	waitForBinding(t, h, "app.mailer", false)
}
