package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnAClassMemberFollowsTheAccessForm(t *testing.T) {
	room := "<?php\nnamespace App;\nclass Room {\n    /** The static size. */\n    public static int $SIZE = 1;\n    /** The constant size. */\n    const SIZE = 2;\n}\necho Room::SIZE;\necho Room::$SIZE;\n"
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///Room.php", room)
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	at := func(needle string) protocol.Position {
		i := strings.Index(room, needle)
		return protocol.Position{Line: strings.Count(room[:i], "\n"), Character: i - strings.LastIndex(room[:i], "\n") - 1}
	}
	for needle, want := range map[string]string{"SIZE;\necho Room::$": "const SIZE = 2", "$SIZE;\n": "The static size."} {
		if h := p.GetHover("file:///Room.php", room, at(needle)); h == nil || !strings.Contains(h.Contents.Value, want) {
			t.Errorf("hover on %q: got %+v, want %q", needle, h, want)
		}
	}
}
