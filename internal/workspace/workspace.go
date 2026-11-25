package workspace

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	frameworkmagento "github.com/Tusk-PHP/lsp/internal/framework/magento"
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
	RootPath  string
	Framework string
	Config    *config.Config
	Logger    *log.Logger

	Index      *symbols.Index
	Container  *container.ContainerAnalyzer
	RouteIndex *frameworklaravel.RouteIndex
	// ViewData indexes the data a Laravel app passes to its views.
	ViewData *frameworklaravel.ViewData
	// Aliases are the Laravel class aliases (facades by short name), read
	// with the Composer dependencies.
	Aliases       *frameworklaravel.Aliases
	SymfonyRoutes []frameworksymfony.Route
	// SymfonyCommands are the project's console commands; nil unless the
	// framework is Symfony. Filled by ScanSymfonyCommands, then kept current
	// file by file with UpdateFile.
	SymfonyCommands *frameworksymfony.CommandIndex
	SchemaCache     *models.SchemaCache
	Schema          *models.Schema

	exclude excludeRules

	profileMu sync.Mutex
	profile   atomic.Pointer[symbols.BuiltinProfile]

	autoloadMu      sync.Mutex
	autoloadRead    bool
	autoloadEntries []composer.AutoloadEntry
	projectFiles    *projectFiles

	// modelMu guards what model analysis read, for re-analysing one file
	// later: Schema once AnalyzeModels has set it, and the following.
	modelMu          sync.RWMutex
	modelsAnalyzed   bool
	migrationColumns map[string]map[string]*models.MigrationColumn
}

func New(opts Options) *Bootstrapped {
	cfg := opts.Config
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	index := symbols.NewIndex()
	index.RegisterBuiltinsForProfile(opts.BuiltinProfile)
	index.SetOptionalNamespaces(composer.SuggestedNamespaces(opts.RootPath))
	if opts.Framework == "magento" {
		index.SetClassGenerator(frameworkmagento.GeneratedClasses{})
	}

	var routeIndex *frameworklaravel.RouteIndex
	var aliases *frameworklaravel.Aliases
	var viewData *frameworklaravel.ViewData
	var commands *frameworksymfony.CommandIndex
	if opts.Framework == "symfony" {
		commands = frameworksymfony.NewCommandIndex()
	}
	if opts.Framework == "laravel" {
		index.SetCallForwards(frameworklaravel.CallForwards())
		index.SetScopeConventions(frameworklaravel.ScopeConventions())
		routeIndex = frameworklaravel.NewRouteIndex(opts.RootPath)
		viewData = frameworklaravel.NewViewData(opts.RootPath)
		aliases = frameworklaravel.NewAliases(opts.RootPath)
		index.SetClassGenerator(aliases)
	}

	ws := &Bootstrapped{
		RootPath:        opts.RootPath,
		Framework:       opts.Framework,
		Config:          cfg,
		Logger:          opts.Logger,
		Index:           index,
		Container:       container.NewContainerAnalyzer(index, opts.RootPath, opts.Framework),
		RouteIndex:      routeIndex,
		ViewData:        viewData,
		Aliases:         aliases,
		SymfonyCommands: commands,
		SchemaCache:     models.NewSchemaCache(),
		exclude:         newExcludeRules(cfg.ExcludePaths),
		projectFiles:    newProjectFiles(cfg.MaxIndexFiles),
	}
	profile := opts.BuiltinProfile
	ws.profile.Store(&profile)
	return ws
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
	ws.ScanSymfonyCommands()
	ws.Index.WarmSearchIndexes()
	ws.Index.MarkReady()
	return ws, nil
}

type IndexedFiles struct {
	Count     int
	Truncated bool
}

func MaxIndexFilesWarning(limit int) string {
	return fmt.Sprintf("Stopped indexing at maxIndexFiles (%d files); classes in the rest of the workspace will be unknown. Set maxIndexFiles to 0 to index everything.", limit)
}

func (ws *Bootstrapped) IndexWorkspace() IndexedFiles {
	if ws == nil {
		return IndexedFiles{}
	}

	ws.logf("Indexing workspace: %s", ws.RootPath)

	ws.projectFiles.begin()
	truncated := false
	var ideHelperFiles []string
	_ = filepath.Walk(ws.RootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if ws.excludedDir(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !IsPHPSource(path) {
			return nil
		}
		// Blade views declare no symbols; only their place is kept, for
		// references and rename, and their PHP is read when it's needed.
		if IsBlade(path) {
			ws.Index.AddTemplateFile("file://" + path)
			if ws.ViewData != nil {
				if content, err := os.ReadFile(path); err == nil {
					ws.ViewData.IndexFile("file://"+path, string(content))
				}
			}
			return nil
		}
		if !ws.projectFiles.admit(path) {
			truncated = true
			ws.logf("%s", MaxIndexFilesWarning(ws.Config.MaxIndexFiles))
			return filepath.SkipAll
		}
		if IsIDEHelperFile(path) {
			ideHelperFiles = append(ideHelperFiles, path)
			return nil
		}
		if content, err := os.ReadFile(path); err == nil {
			ws.Index.IndexFileWithSource("file://"+path, string(content), symbols.SourceProject)
			ws.ViewData.IndexFile("file://"+path, string(content))
		}
		return nil
	})

	for _, path := range ideHelperFiles {
		if content, err := os.ReadFile(path); err == nil {
			ws.Index.IndexIDEHelperFile("file://"+path, string(content))
			ws.logf("Indexed IDE helper: %s", filepath.Base(path))
		}
	}

	count := ws.projectFiles.finish()
	ws.logf("Indexed %d PHP files", count)
	return IndexedFiles{Count: count, Truncated: truncated}
}

func (ws *Bootstrapped) excludedDir(path string) bool {
	rel, err := filepath.Rel(ws.RootPath, path)
	if err != nil || rel == "." {
		return false
	}
	return ws.exclude.excludes(filepath.ToSlash(rel))
}

func (ws *Bootstrapped) IndexComposerDependencies() int {
	if ws == nil {
		return 0
	}

	if ws.Aliases != nil {
		ws.Aliases.Load()
	}

	entries := ws.autoload()
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
	ws.AnalyzeModelClasses()
	ws.AnalyzeModelSchema()
}

func (ws *Bootstrapped) AnalyzeModelClasses() {
	if ws == nil {
		return
	}
	switch ws.Framework {
	case "laravel":
		models.AnalyzeEloquentModels(ws.Index, ws.RootPath)
	case "symfony":
		models.AnalyzeDoctrineEntities(ws.Index, ws.RootPath)
	}
}

func (ws *Bootstrapped) AnalyzeModelSchema() {
	if ws == nil {
		return
	}
	schema, _ := models.ScanSchema(ws.RootPath, ws.Framework, ws.Config, ws.Logger, ws.SchemaCache)
	models.InjectDatabaseSchema(ws.Index, ws.RootPath, ws.Framework, schema)
	var migrationColumns map[string]map[string]*models.MigrationColumn
	if ws.Framework == "laravel" && (schema == nil || schema.Source != models.SchemaSourceMigrations) {
		migrationColumns = models.ScanMigrationColumns(ws.RootPath)
		models.InjectMigrationColumns(ws.Index, ws.RootPath, migrationColumns)
	}
	ws.modelMu.Lock()
	ws.Schema = schema
	ws.migrationColumns = migrationColumns
	ws.modelsAnalyzed = true
	ws.modelMu.Unlock()
}

// ScanSymfonyCommands finds the console commands the project declares. It
// reads every indexed project file, so it runs once the workspace is indexed,
// off any request path.
func (ws *Bootstrapped) ScanSymfonyCommands() {
	if ws == nil || ws.SymfonyCommands == nil {
		return
	}
	ws.SymfonyCommands.ReplaceWith(frameworksymfony.DiscoverCommands(ws.Index))
}

// UpdateFile keeps what the workspace derives file by file current after
// uri was re-indexed with source: the models it declares and its console
// commands. Its work grows with the file only.
func (ws *Bootstrapped) UpdateFile(uri, source string) {
	if ws == nil {
		return
	}
	ws.AnalyzeModelFile(uri, source)
	ws.SymfonyCommands.UpdateFile(uri, source)
}

// AnalyzeModelFile re-runs model analysis for the models declared in one file
// after it was re-indexed, which drops the members model analysis added to
// them (relations, accessors, database columns). source is the file's
// current text. It does nothing until AnalyzeModels has run, and its work
// grows with the file only.
func (ws *Bootstrapped) AnalyzeModelFile(uri, source string) {
	if ws == nil {
		return
	}
	ws.modelMu.RLock()
	analyzed, schema, migrationColumns := ws.modelsAnalyzed, ws.Schema, ws.migrationColumns
	ws.modelMu.RUnlock()
	if !analyzed {
		return
	}
	models.AnalyzeModelFile(ws.Index, ws.RootPath, ws.Framework, uri, source, schema, migrationColumns)
}

// SchemaIsLive reports whether the database columns given to models were
// read from a live database, which lists every column of a table, rather
// than worked out from migrations, which can miss some.
func (ws *Bootstrapped) SchemaIsLive() bool {
	if ws == nil {
		return false
	}
	ws.modelMu.RLock()
	defer ws.modelMu.RUnlock()
	return ws.Schema != nil && ws.Schema.Source == models.SchemaSourceLive
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
