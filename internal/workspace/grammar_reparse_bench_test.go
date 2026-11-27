package workspace

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/config"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func BenchmarkReparseForGrammars(b *testing.B) {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "laravel"))
	if err != nil {
		b.Fatal(err)
	}
	cfg := config.DefaultConfig()
	off := false
	cfg.DatabaseEnabled = &off
	profile, _ := ResolveBuiltinProfileWithConfig(root, "", time.Second, cfg, nil)
	ws := New(Options{RootPath: root, Framework: "laravel", Config: cfg, Logger: log.New(io.Discard, "", 0), BuiltinProfile: profile})
	if err := ws.Load(context.Background()); err != nil {
		b.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		ranges [2]phpparser.GrammarRange
	}{
		{"every file changes grammar", [2]phpparser.GrammarRange{phpparser.OnlyGrammar(token.PHP74), profile.Grammars}},
		{"the newest grammar stays", [2]phpparser.GrammarRange{phpparser.GrammarsBetween(token.PHP84, token.Latest), profile.Grammars}},
	} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			files := 0
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				next := profile
				next.Grammars = c.ranges[i%2]
				if _, err := ws.SetProfile(next); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				files += ws.ReparseForGrammars()
			}
			b.ReportMetric(float64(files)/float64(b.N), "files/op")
		})
	}
}
