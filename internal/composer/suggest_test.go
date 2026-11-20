package composer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuggestedNamespaces(t *testing.T) {
	root := t.TempDir()
	composerJSON := `{"suggest": {
		"ext-redis": "Redis cache",
		"ably/ably-php": "Ably broadcasting",
		"pusher/pusher-php-server": "Pusher broadcasting",
		"predis/predis": "Redis client",
		"intervention/image": "Images",
		"symfony/mercure": "Mercure"
	}}`
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(composerJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	// predis is installed, so its classes are indexed and a miss is real.
	if err := os.MkdirAll(filepath.Join(root, "vendor", "predis", "predis"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(SuggestedNamespaces(root), " ")
	want := `ably\ ably\ablyphp\ image\ intervention\image\ mercure\ pusher\ pusher\pusherphpserver\ symfony\mercure\`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if SuggestedNamespaces(t.TempDir()) != nil {
		t.Error("a project without composer.json suggests nothing")
	}
}
