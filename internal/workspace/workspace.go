package workspace

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type Options struct {
	RootPath       string
	Framework      string
	Config         *config.Config
	Logger         *log.Logger
	BuiltinProfile symbols.BuiltinProfile
}

type Bootstrapped struct {
	RootPath       string
	Framework      string
	Config         *config.Config
	Logger         *log.Logger
	BuiltinProfile symbols.BuiltinProfile

	Index          *symbols.Index
	Container      *container.ContainerAnalyzer
	RouteIndex     *frameworklaravel.RouteIndex
	SymfonyRoutes  []frameworksymfony.Route
	SchemaCache    *models.SchemaCache
	Schema         *models.Schema
}

func New(opts Options) *Bootstrapped {
	cfg := opts.Config
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	index := symbols.NewIndex()
	index.RegisterBuiltinsForProfile(opts.BuiltinProfile)

	var routeIndex *frameworklaravel.RouteIndex
	if opts.Framework == "laravel" {
		routeIndex = frameworklaravel.NewRouteIndex(opts.RootPath)
	}

	return &Bootstrapped{
		RootPath:       opts.RootPath,
		Framework:      opts.Framework,
		Config:         cfg,
		Logger:         opts.Logger,
		BuiltinProfile: opts.BuiltinProfile,
		Index:          index,
		Container:      container.NewContainerAnalyzer(index, opts.RootPath, opts.Framework),
		RouteIndex:     routeIndex,
		SchemaCache:    models.NewSchemaCache(),
	}
}

func Build(ctx context.Context, opts Options) (*Bootstrapped, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	ws := New(opts)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ws.IndexWorkspace()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ws.IndexComposerDependencies()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ws.Container.Analyze()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ws.ScanRoutes(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ws.AnalyzeModels()
	ws.Index.WarmSearchIndexes()
	ws.Index.MarkReady()
	return ws, nil
}

func (ws *Bootstrapped) IndexWorkspace() int {
	if ws == nil {
		return 0
	}

	ws.logf("Indexing workspace: %s", ws.RootPath)

	count := 0
	var ideHelperFiles []string
	_ = filepath.Walk(ws.RootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			for _, ex := range ws.Config.ExcludePaths {
				if filepath.Base(path) == ex {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".php") {
			return nil
		}
		if count >= ws.Config.MaxIndexFiles {
			return filepath.SkipAll
		}
		if IsIDEHelperFile(path) {
			ideHelperFiles = append(ideHelperFiles, path)
			return nil
		}
		if content, err := os.ReadFile(path); err == nil {
			ws.Index.IndexFileWithSource("file://"+path, string(content), symbols.SourceProject)
			count++
		}
		return nil
	})

	for _, path := range ideHelperFiles {
		if content, err := os.ReadFile(path); err == nil {
			ws.Index.IndexIDEHelperFile("file://"+path, string(content))
			count++
			ws.logf("Indexed IDE helper: %s", filepath.Base(path))
		}
	}

	ws.logf("Indexed %d PHP files", count)
	return count
}

func (ws *Bootstrapped) IndexComposerDependencies() int {
	if ws == nil {
		return 0
	}

	entries := composer.GetAutoloadPaths(ws.RootPath)
	if len(entries) == 0 {
		return 0
	}

	vendorCount := 0
	for _, entry := range entries {
		src := symbols.SourceProject
		if entry.IsVendor {
			src = symbols.SourceVendor
		}

		if entry.IsFile {
			if content, err := os.ReadFile(entry.Path); err == nil {
				ws.Index.IndexFileWithSource("file://"+entry.Path, string(content), src)
				if entry.IsVendor {
					vendorCount++
				}
			}
			continue
		}

		if !entry.IsVendor {
			continue
		}
		info, err := os.Stat(entry.Path)
		if err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.Walk(entry.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".php") {
				return nil
			}
			if content, err := os.ReadFile(path); err == nil {
				ws.Index.IndexFileWithSource("file://"+path, string(content), symbols.SourceVendor)
				vendorCount++
			}
			return nil
		})
	}

	ws.logf("Indexed %d vendor PHP files from Composer dependencies", vendorCount)
	return vendorCount
}

func (ws *Bootstrapped) ScanRoutes() error {
	if ws == nil {
		return nil
	}
	if ws.RouteIndex != nil {
		if err := ws.RouteIndex.ScanWorkspace(); err != nil {
			return err
		}
	}
	if ws.Framework == "symfony" {
		ws.SymfonyRoutes = frameworksymfony.DiscoverRoutes(ws.Index)
	}
	return nil
}

func (ws *Bootstrapped) AnalyzeModels() {
	if ws == nil {
		return
	}

	switch ws.Framework {
	case "laravel":
		models.AnalyzeEloquentModels(ws.Index, ws.RootPath)
	case "symfony":
		models.AnalyzeDoctrineEntities(ws.Index, ws.RootPath)
	}

	ws.Schema, _ = models.ScanSchema(ws.RootPath, ws.Framework, ws.Config, ws.Logger, ws.SchemaCache)
	models.InjectDatabaseSchema(ws.Index, ws.RootPath, ws.Framework, ws.Schema)
	if ws.Framework == "laravel" && (ws.Schema == nil || ws.Schema.Source != models.SchemaSourceMigrations) {
		models.AnalyzeMigrations(ws.Index, ws.RootPath)
	}
}

func IsIDEHelperFile(path string) bool {
	base := filepath.Base(path)
	return base == "_ide_helper_models.php" || base == "_ide_helper.php"
}

func (ws *Bootstrapped) logf(format string, args ...any) {
	if ws != nil && ws.Logger != nil {
		ws.Logger.Printf(format, args...)
	}
}
