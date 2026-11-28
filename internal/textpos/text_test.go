package textpos

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoreTextTurnsLoneCRsIntoNewlines(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"", ""},
		{"a\nb\r\nc", "a\nb\r\nc"},
		{"a\rb\r", "a\nb\n"},
		{"a\r\r\nb\rc\r\n", "a\n\r\nb\nc\r\n"},
	} {
		got := CoreText(c.text)
		if got != c.want || len(got) != len(c.text) {
			t.Errorf("CoreText(%q) = %q, want %q of the same length", c.text, got, c.want)
		}
	}
}

func TestReadTextReturnsCoreText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.php")
	if err := os.WriteFile(path, []byte("<?php\rfoo();\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadText(path); err != nil || got != "<?php\nfoo();\r\n" {
		t.Errorf("ReadText = %q, %v", got, err)
	}
	if _, err := ReadText(filepath.Join(t.TempDir(), "missing.php")); err == nil {
		t.Error("reading a missing file did not fail")
	}
}
