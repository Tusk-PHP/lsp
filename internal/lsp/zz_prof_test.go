package lsp

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// ZZ_ROOT, ZZ_GLOB (comma list, ** not supported), ZZ_OUT (diagnostics dump).
func TestZZProfileDiagnostics(t *testing.T) {
	root := os.Getenv("ZZ_ROOT")
	if root == "" {
		t.Skip()
	}
	ws, err := workspace.Build(context.Background(), workspace.Options{RootPath: root, BuiltinProfile: symbols.DefaultBuiltinProfile()})
	if err != nil {
		t.Fatal(err)
	}
	comp := completion.NewProvider(ws.Index, ws.Container, "")
	d := diagnostics.NewProvider(ws.Index, "", root, log.New(io.Discard, "", 0), config.DefaultConfig())
	d.TypeResolver = func(expr, source string, line int, file *parser.FileNode) string {
		return comp.ResolveDiagnosticType(expr, source, protocol.Position{Line: line}, file)
	}
	var files []string
	for _, g := range strings.Split(os.Getenv("ZZ_GLOB"), ",") {
		m, _ := filepath.Glob(filepath.Join(root, g))
		files = append(files, m...)
	}
	sort.Strings(files)
	var out []string
	start := time.Now()
	for _, f := range files {
		b, _ := os.ReadFile(f)
		t0 := time.Now()
		for _, dg := range d.Analyze("file://"+f, string(b)) {
			out = append(out, fmt.Sprintf("%s:%d:%d %v %s", strings.TrimPrefix(f, root), dg.Range.Start.Line, dg.Range.Start.Character, dg.Code, dg.Message))
		}
		if el := time.Since(t0); el > 500*time.Millisecond {
			t.Logf("slow %s %s", el, strings.TrimPrefix(f, root))
		}
	}
	t.Logf("%d files, %d diags, %s", len(files), len(out), time.Since(start))
	if p := os.Getenv("ZZ_OUT"); p != "" {
		os.WriteFile(p, []byte(strings.Join(out, "\n")+"\n"), 0o644)
	}
}
