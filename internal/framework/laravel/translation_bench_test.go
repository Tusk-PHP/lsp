package laravel

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func benchmarkTranslations(b *testing.B) *TranslationResolver {
	b.Helper()
	root := b.TempDir()
	var file strings.Builder
	file.WriteString("<?php\nreturn [\n")
	for k := range 50 {
		file.WriteString("    'key" + strconv.Itoa(k) + "' => 'Value',\n")
	}
	file.WriteString("];\n")
	for _, locale := range []string{"en", "de", "fr", "es", "it", "nl", "pt", "da"} {
		dir := filepath.Join(root, "lang", locale)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		for f := range 20 {
			if err := os.WriteFile(filepath.Join(dir, "group"+strconv.Itoa(f)+".php"), []byte(file.String()), 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	return NewTranslationResolver(root)
}

func BenchmarkTranslationComplete(b *testing.B) {
	r := benchmarkTranslations(b)
	ctx := &TranslationStringContext{Partial: "group1"}
	b.ReportAllocs()
	for b.Loop() {
		if len(r.Matching(ctx)) == 0 {
			b.Fatal("no keys completed")
		}
	}
}

func BenchmarkTranslationDefinition(b *testing.B) {
	r := benchmarkTranslations(b)
	b.ReportAllocs()
	for b.Loop() {
		if r.Definition("group19.key49") == nil {
			b.Fatal("no definition")
		}
	}
}
