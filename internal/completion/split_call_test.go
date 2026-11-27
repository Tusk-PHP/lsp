package completion

import (
	"path/filepath"
	"strings"
	"testing"

	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func completeMarkedItems(p *Provider, marked string) []protocol.CompletionItem {
	before, after, _ := strings.Cut(marked, "‸")
	pos := protocol.Position{Line: strings.Count(before, "\n"), Character: len(before) - strings.LastIndex(before, "\n") - 1}
	return p.GetCompletions("file:///test.php", before+after, pos)
}

func completeMarked(p *Provider, marked string) map[string]bool {
	return collectLabels(completeMarkedItems(p, marked))
}

func itemLabeled(items []protocol.CompletionItem, label string) (protocol.CompletionItem, bool) {
	for _, item := range items {
		if item.Label == label {
			return item, true
		}
	}
	return protocol.CompletionItem{}, false
}

func TestCompletionContextsSplitOverLines(t *testing.T) {
	p := setupCoverageCompletion()
	p.index.IndexFile("file:///test.php", "<?php\n")
	tests := []struct {
		name  string
		code  string
		label string
		want  func(item protocol.CompletionItem, found bool, labels map[string]bool) bool
	}{
		{name: "new on the line before the class name", code: "<?php\nreturn new\n    Ser‸", label: "Service",
			want: func(item protocol.CompletionItem, found bool, _ map[string]bool) bool {
				return found && item.InsertText == "Service($0)"
			}},
		{name: "a group use split over lines", code: "<?php\nuse App\\{\n    ‸\n};", label: "Service",
			want: func(_ protocol.CompletionItem, found bool, labels map[string]bool) bool {
				return found && labels["Controller"] && !labels["Logger"]
			}},
		{name: "an attribute split over lines", code: "<?php\n#[\n    Ove‸\n]\nfunction f() {}", label: "Override",
			want: func(item protocol.CompletionItem, found bool, _ map[string]bool) bool {
				return found && item.Detail == "PHP 8.3"
			}},
		{name: "a pipe on the line before", code: "<?php\n$x = 'a'\n    |>\n    ‸", label: "strlen",
			want: func(_ protocol.CompletionItem, found bool, labels map[string]bool) bool {
				return found && !labels["pi"] && !labels["return"]
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := completeMarkedItems(p, tt.code)
			item, found := itemLabeled(items, tt.label)
			if labels := collectLabels(items); !tt.want(item, found, labels) {
				t.Errorf("%q: got %+v, %v among %v", tt.label, item, found, mapKeys(labels, 10))
			}
		})
	}
}

func TestArrayKeysOfALevelOnTheNextLine(t *testing.T) {
	p := NewProvider(symbols.NewIndex(), nil, "none")
	labels := completeMarked(p, "<?php\n$config = ['database' => ['replicas' => 1, 'primary' => 2]];\n$config['database']\n    ['rep‸")
	if !labels["replicas"] || labels["primary"] {
		t.Errorf("got %v", mapKeys(labels, 10))
	}
}

func TestTranslationKeysOnTheNextLine(t *testing.T) {
	p := setupTranslationProvider(t)
	if labels := completeMarked(p, "<?php\necho __(\n    'mes‸"); !labels["messages.welcome"] {
		t.Errorf("Laravel: got %v", mapKeys(labels, 10))
	}

	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "symfony"))
	if err != nil {
		t.Fatal(err)
	}
	sf := NewProvider(symbols.NewIndex(), nil, "symfony")
	sf.SetSymfonyTranslationResolver(frameworksymfony.NewTranslationResolver(root))
	if labels := completeMarked(sf, "<?php\n$translator->trans(\n    'nav.‸"); !labels["nav.home"] {
		t.Errorf("Symfony: got %v", mapKeys(labels, 10))
	}
	if labels := completeMarked(sf, "<?php\n// $translator->trans('nav.‸"); labels["nav.home"] {
		t.Error("Symfony: a translation key in a comment")
	}
}
