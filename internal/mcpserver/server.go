package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/hover"
	"github.com/Tusk-PHP/lsp/internal/inlayhint"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/symquery"
	"github.com/Tusk-PHP/lsp/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ServerName = "tusk-mcp"

// ServerVersion is overridden at startup from the binary's stamped version.
var ServerVersion = "0.5.0"

// ContractVersion is the stable wire-format version for MCP tool contracts.
// Increment when tool schemas or output shapes change incompatibly.
const ContractVersion = 1

type Service struct {
	RootPath         string
	Framework        string
	Config           *config.Config
	BuiltinProfile   symbols.BuiltinProfile
	BuiltinPHPSource string

	// Workspace is kept for tests and compatibility, but serving reads go
	// through runtime() so reindex can atomically swap the full immutable state.
	Workspace    *workspace.Bootstrapped
	Analyzer     *analyzer.Analyzer
	Completion   *completion.Provider
	Hover        *hover.Provider
	InlayHint    *inlayhint.Provider
	Diagnostics  *diagnostics.Provider
	Server       *mcp.Server
	runtimeState atomic.Pointer[serviceRuntime]

	projectSummaryURI string
	dbSchemaURI       string
	phpSymbolsURI     string
	laravelRoutesURI  string
	laravelModelsURI  string
	symfonyRoutesURI  string
}

type serviceRuntime struct {
	Workspace   *workspace.Bootstrapped
	Analyzer    *analyzer.Analyzer
	Completion  *completion.Provider
	Hover       *hover.Provider
	InlayHint   *inlayhint.Provider
	Diagnostics *diagnostics.Provider
}

type ReindexOutput struct {
	RootPath       string   `json:"rootPath"`
	Framework      string   `json:"framework"`
	IndexedFiles   int      `json:"indexedFiles"`
	RouteCount     int      `json:"routeCount"`
	SchemaSource   string   `json:"schemaSource"`
	SchemaTables   []string `json:"schemaTables,omitempty"`
	PHPVersion     string   `json:"phpVersion"`
	PHPVersionFrom string   `json:"phpVersionSource"`
}

type ProjectSummary struct {
	RootPath         string   `json:"rootPath"`
	Framework        string   `json:"framework"`
	PHPVersion       string   `json:"phpVersion"`
	PHPVersionSource string   `json:"phpVersionSource"`
	IndexedFiles     int      `json:"indexedFiles"`
	RouteCount       int      `json:"routeCount"`
	SchemaSource     string   `json:"schemaSource,omitempty"`
	SchemaTables     []string `json:"schemaTables,omitempty"`
	ContractVersion  int      `json:"contractVersion"`
}

type SymbolMatch struct {
	Name   string `json:"name"`
	FQN    string `json:"fqn"`
	Kind   string `json:"kind"`
	URI    string `json:"uri,omitempty"`
	Source string `json:"source"`
}

type SymbolMember struct {
	Name       string `json:"name"`
	FQN        string `json:"fqn"`
	Kind       string `json:"kind"`
	Type       string `json:"type,omitempty"`
	ReturnType string `json:"returnType,omitempty"`
	Virtual    bool   `json:"virtual,omitempty"`
}

type SymbolExplanation struct {
	Symbol                *SymbolMatch   `json:"symbol,omitempty"`
	Type                  string         `json:"type,omitempty"`
	ReturnType            string         `json:"returnType,omitempty"`
	DocComment            string         `json:"docComment,omitempty"`
	InheritanceChain      []string       `json:"inheritanceChain,omitempty"`
	ImplementedInterfaces []string       `json:"implementedInterfaces,omitempty"`
	MemberCount           int            `json:"memberCount,omitempty"`
	Members               []SymbolMember `json:"members,omitempty"`
	Descendants           []string       `json:"descendants,omitempty"`
	Implementors          []string       `json:"implementors,omitempty"`
	NamespaceMembers      []SymbolMatch  `json:"namespaceMembers,omitempty"`
}

type FindSymbolInput struct {
	Query string `json:"query" jsonschema:"symbol name or FQN prefix"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results to return"`
}

type FindSymbolOutput struct {
	Query   string        `json:"query"`
	Results []SymbolMatch `json:"results"`
}

type ExplainSymbolInput struct {
	FQN string `json:"fqn" jsonschema:"fully qualified symbol name"`
}

type FindReferencesInput struct {
	FQN string `json:"fqn" jsonschema:"fully qualified symbol name"`
}

type ReferenceLocation struct {
	URI   string         `json:"uri"`
	Range protocol.Range `json:"range"`
}

type FindReferencesOutput struct {
	FQN        string              `json:"fqn"`
	References []ReferenceLocation `json:"references"`
}

type DiagnosticsInput struct {
	Path string `json:"path,omitempty" jsonschema:"optional workspace-relative or absolute PHP file path"`
}

type FileDiagnostics struct {
	Path        string                `json:"path"`
	Diagnostics []protocol.Diagnostic `json:"diagnostics"`
}

type DiagnosticsOutput struct {
	Files []FileDiagnostics `json:"files"`
}

type ListTablesOutput struct {
	Connection string   `json:"connection,omitempty"`
	Database   string   `json:"database,omitempty"`
	Source     string   `json:"source,omitempty"`
	Connected  bool     `json:"connected"`
	Caveat     string   `json:"caveat,omitempty"`
	Tables     []string `json:"tables"`
}

type DescribeTableInput struct {
	Name string `json:"name" jsonschema:"database table name"`
}

type DescribeTableOutput struct {
	Connection string              `json:"connection,omitempty"`
	Database   string              `json:"database,omitempty"`
	Source     string              `json:"source,omitempty"`
	Connected  bool                `json:"connected"`
	Caveat     string              `json:"caveat,omitempty"`
	Table      *models.SchemaTable `json:"table,omitempty"`
}

type FindColumnInput struct {
	Name string `json:"name" jsonschema:"column name"`
}

type ColumnMatch struct {
	Table  string              `json:"table"`
	Column models.SchemaColumn `json:"column"`
}

type FindColumnOutput struct {
	Connection string        `json:"connection,omitempty"`
	Database   string        `json:"database,omitempty"`
	Source     string        `json:"source,omitempty"`
	Connected  bool          `json:"connected"`
	Caveat     string        `json:"caveat,omitempty"`
	Matches    []ColumnMatch `json:"matches"`
}

type CompactSchemaDocument struct {
	Source     string                       `json:"source"`
	Connection string                       `json:"connection,omitempty"`
	Database   string                       `json:"database,omitempty"`
	Connected  bool                         `json:"connected"`
	Caveat     string                       `json:"caveat,omitempty"`
	Tables     map[string]map[string]string `json:"tables"`
}

type LaravelRouteRecord struct {
	Name              string          `json:"name"`
	Path              string          `json:"path,omitempty"`
	URI               string          `json:"uri"`
	Range             protocol.Range  `json:"range"`
	TargetKind        string          `json:"targetKind,omitempty"`
	Target            string          `json:"target,omitempty"`
	HandlerFQN        string          `json:"handlerFqn,omitempty"`
	HandlerMethod     string          `json:"handlerMethod,omitempty"`
	HandlerDefinition *SymbolMatch    `json:"handlerDefinition,omitempty"`
	HandlerRange      *protocol.Range `json:"handlerRange,omitempty"`
}

type LaravelRoutesOutput struct {
	Routes []LaravelRouteRecord `json:"routes"`
}

type LaravelRouteInput struct {
	Name string `json:"name" jsonschema:"Laravel route name"`
}

type LaravelModelInput struct {
	FQN string `json:"fqn" jsonschema:"fully qualified Laravel model class"`
}

type LaravelModelSchemaOutput struct {
	Model       *SymbolMatch          `json:"model,omitempty"`
	TableName   string                `json:"tableName,omitempty"`
	Table       *models.SchemaTable   `json:"table,omitempty"`
	Members     []SymbolMember        `json:"members,omitempty"`
	MemberCount int                   `json:"memberCount,omitempty"`
	Relations   []ModelRelationRecord `json:"relations,omitempty"`
}

// ModelRelationRecord is a single resolved Eloquent/Doctrine relation declared
// on a model, returned by the laravel_model_schema MCP tool.
type ModelRelationRecord struct {
	Method      string `json:"method"`
	Kind        string `json:"kind"`
	Target      string `json:"target,omitempty"`
	Cardinality string `json:"cardinality,omitempty"`
}

type PhpContainerBindingsInput struct {
	Class string `json:"class,omitempty" jsonschema:"optional fully qualified class to resolve constructor injection for"`
}

// ContainerBindingRecord is a single resolved DI container binding returned by
// the php_container_bindings MCP tool.
type ContainerBindingRecord struct {
	Abstract      string         `json:"abstract"`
	Concrete      string         `json:"concrete,omitempty"`
	Singleton     bool           `json:"singleton,omitempty"`
	Source        string         `json:"source,omitempty"`
	Alias         string         `json:"alias,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	DefinitionURI string         `json:"definitionUri,omitempty"`
	Range         protocol.Range `json:"range"`
}

// ConstructorInjectionRecord describes one resolved constructor parameter for a
// class, returned by the php_container_bindings MCP tool when `class` is set.
type ConstructorInjectionRecord struct {
	ParamName        string `json:"paramName"`
	TypeHint         string `json:"typeHint,omitempty"`
	ResolvedConcrete string `json:"resolvedConcrete,omitempty"`
	IsSingleton      bool   `json:"isSingleton,omitempty"`
	AutowireKind     string `json:"autowireKind,omitempty"`
	AutowireValue    string `json:"autowireValue,omitempty"`
}

type PhpContainerBindingsOutput struct {
	Bindings  []ContainerBindingRecord     `json:"bindings"`
	Injection []ConstructorInjectionRecord `json:"injection,omitempty"`
}

// SymfonyRouteRecord is a single Symfony route entry returned by the MCP tools.
type SymfonyRouteRecord struct {
	Name          string         `json:"name"`
	Path          string         `json:"path,omitempty"`
	URI           string         `json:"uri,omitempty"`
	DeclRange     protocol.Range `json:"declRange"`
}

type SymfonyRoutesOutput struct {
	Routes []SymfonyRouteRecord `json:"routes"`
}

type SymfonyRouteInput struct {
	Name string `json:"name" jsonschema:"Symfony route name"`
}


func New(ctx context.Context, rootPath string, logger *slog.Logger) (*Service, error) {
	if rootPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		rootPath = cwd
	}
	rootPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	cfg, framework, profile, source, err := loadProjectMetadata(rootPath, logger)
	if err != nil {
		return nil, err
	}
	ws, err := workspace.Build(ctx, workspace.Options{
		RootPath:       rootPath,
		Framework:      framework,
		Config:         cfg,
		Logger:         newStdLogger(logger),
		BuiltinProfile: profile,
	})
	if err != nil {
		return nil, err
	}

	svc := &Service{
		RootPath:          rootPath,
		Framework:         framework,
		Config:            cfg,
		BuiltinProfile:    profile,
		BuiltinPHPSource:  source,
		Workspace:         ws,
		projectSummaryURI: "tusk://project/summary",
		dbSchemaURI:       "tusk://db/schema/compact",
		phpSymbolsURI:     "tusk://php/symbols",
		laravelRoutesURI:  "tusk://laravel/routes",
		laravelModelsURI:  "tusk://laravel/models",
		symfonyRoutesURI:  "tusk://symfony/routes",
	}
	svc.installRuntime(svc.buildRuntime(ws))

	svc.Server = mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: ServerVersion}, &mcp.ServerOptions{
		Logger: logger,
	})
	svc.registerTools()
	svc.registerResources()

	return svc, nil
}

func (s *Service) Run(ctx context.Context) error {
	return s.Server.Run(ctx, &mcp.StdioTransport{})
}

func (s *Service) Dump(ctx context.Context, outDir string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if outDir == "" {
		outDir = filepath.Join(s.RootPath, ".tusk", "ai-context")
	}
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(s.RootPath, outDir)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	files := map[string]any{
		"db-schema.compact.json": compactSchemaDocument(s.runtime().Workspace.Schema),
		"symbols.json":           s.symbolCatalog(),
	}
	if err := writeFile(filepath.Join(outDir, "project-summary.md"), []byte(s.projectSummaryMarkdown())); err != nil {
		return err
	}
	for name, value := range files {
		if err := writeJSONFile(filepath.Join(outDir, name), value); err != nil {
			return err
		}
	}
	if s.Framework == "laravel" {
		if err := writeJSONFile(filepath.Join(outDir, "laravel-routes.json"), s.laravelRoutes()); err != nil {
			return err
		}
		if err := writeJSONFile(filepath.Join(outDir, "models.json"), s.laravelModels()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) registerTools() {
	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "php_project_summary",
		Description: "Return a concise summary of the PHP project, framework, indexed files, and live schema availability.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ProjectSummary, error) {
		return nil, s.projectSummary(), nil
	})

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "php_find_symbol",
		Description: "Find PHP symbols by exact name, name prefix, or FQN prefix.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in FindSymbolInput) (*mcp.CallToolResult, FindSymbolOutput, error) {
		return nil, FindSymbolOutput{
			Query:   in.Query,
			Results: s.findSymbols(in.Query, in.Limit),
		}, nil
	})

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "php_explain_symbol",
		Description: "Explain a PHP symbol by FQN, including members, inheritance, and interfaces where applicable.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in ExplainSymbolInput) (*mcp.CallToolResult, SymbolExplanation, error) {
		sym := s.runtime().Workspace.Index.Lookup(in.FQN)
		if sym == nil {
			return nil, SymbolExplanation{}, fmt.Errorf("symbol %q not found", in.FQN)
		}
		return nil, s.explainSymbol(sym), nil
	})

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "php_find_references",
		Description: "Find references for a PHP symbol by FQN.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in FindReferencesInput) (*mcp.CallToolResult, FindReferencesOutput, error) {
		refs, err := s.findReferences(in.FQN)
		if err != nil {
			return nil, FindReferencesOutput{}, err
		}
		return nil, FindReferencesOutput{FQN: in.FQN, References: refs}, nil
	})

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "php_diagnostics",
		Description: "Run native in-process diagnostics for one PHP file or the indexed project. Excludes external tools like PHPStan and Pint.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in DiagnosticsInput) (*mcp.CallToolResult, DiagnosticsOutput, error) {
		out, err := s.projectDiagnostics(in.Path)
		if err != nil {
			return nil, DiagnosticsOutput{}, err
		}
		return nil, out, nil
	})

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "tusk_reindex",
		Description: "Rebuild the MCP workspace snapshot and atomically swap it into service.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ReindexOutput, error) {
		out, err := s.Reindex(ctx)
		if err != nil {
			return nil, ReindexOutput{}, err
		}
		return nil, out, nil
	})

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "db_list_tables",
		Description: "List tables from the normalized live database schema snapshot.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ListTablesOutput, error) {
		schema := s.runtime().Workspace.Schema
		if schema == nil {
			return nil, ListTablesOutput{Tables: []string{}}, nil
		}
		return nil, ListTablesOutput{
			Connection: schema.Connection,
			Database:   schema.Database,
			Source:     schema.Source,
			Connected:  schema.Connected,
			Caveat:     schema.Caveat,
			Tables:     sortedStrings(schema.TableNames()),
		}, nil
	})

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "db_describe_table",
		Description: "Describe a live database table using the normalized schema snapshot.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in DescribeTableInput) (*mcp.CallToolResult, DescribeTableOutput, error) {
		schema := s.runtime().Workspace.Schema
		if schema == nil {
			return nil, DescribeTableOutput{}, fmt.Errorf("database schema is unavailable")
		}
		table := schema.Table(in.Name)
		if table == nil {
			return nil, DescribeTableOutput{}, fmt.Errorf("table %q not found", in.Name)
		}
		out := DescribeTableOutput{
			Connection: schema.Connection,
			Database:   schema.Database,
			Source:     schema.Source,
			Connected:  schema.Connected,
			Caveat:     schema.Caveat,
			Table:      table,
		}
		return nil, out, nil
	})

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "db_find_column",
		Description: "Find a database column across all known tables in the normalized schema snapshot.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in FindColumnInput) (*mcp.CallToolResult, FindColumnOutput, error) {
		return nil, s.findColumn(in.Name), nil
	})

	if s.Framework == "laravel" {
		mcp.AddTool(s.Server, &mcp.Tool{
			Name:        "laravel_routes",
			Description: "List known Laravel route names and their definition locations.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, LaravelRoutesOutput, error) {
			return nil, LaravelRoutesOutput{Routes: s.laravelRoutes()}, nil
		})

		mcp.AddTool(s.Server, &mcp.Tool{
			Name:        "laravel_route_to_controller",
			Description: "Best-effort Laravel route definition lookup by route name. Returns the route definition location; controller extraction is not available in the current index.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, in LaravelRouteInput) (*mcp.CallToolResult, *LaravelRouteRecord, error) {
			record := s.findLaravelRoute(in.Name)
			if record == nil {
				return nil, nil, fmt.Errorf("route %q not found", in.Name)
			}
			return nil, record, nil
		})

		mcp.AddTool(s.Server, &mcp.Tool{
			Name:        "laravel_model_schema",
			Description: "Describe a Laravel Eloquent model, its resolved table, and known members.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, in LaravelModelInput) (*mcp.CallToolResult, LaravelModelSchemaOutput, error) {
			out, err := s.laravelModelSchema(in.FQN)
			if err != nil {
				return nil, LaravelModelSchemaOutput{}, err
			}
			return nil, out, nil
		})
	}

	// php_container_bindings is framework-agnostic: it surfaces resolved DI
	// container bindings for both Laravel and Symfony workspaces.
	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "php_container_bindings",
		Description: "List resolved DI container bindings (interface→concrete, singleton, source, alias, tags). Optional `class` returns that class's resolved constructor injection.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in PhpContainerBindingsInput) (*mcp.CallToolResult, PhpContainerBindingsOutput, error) {
		return nil, s.phpContainerBindings(in.Class), nil
	})

	if s.Framework == "symfony" {
		mcp.AddTool(s.Server, &mcp.Tool{
			Name:        "symfony_routes",
			Description: "List known Symfony route names, paths, and declaration file locations.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, SymfonyRoutesOutput, error) {
			return nil, SymfonyRoutesOutput{Routes: s.symfonyRoutes()}, nil
		})

		mcp.AddTool(s.Server, &mcp.Tool{
			Name:        "symfony_route_to_controller",
			Description: "Look up a Symfony route by name and return its declaration location.",
		}, func(_ context.Context, _ *mcp.CallToolRequest, in SymfonyRouteInput) (*mcp.CallToolResult, *SymfonyRouteRecord, error) {
			record := s.findSymfonyRoute(in.Name)
			if record == nil {
				return nil, nil, fmt.Errorf("route %q not found", in.Name)
			}
			return nil, record, nil
		})
	}

}

func (s *Service) registerResources() {
	s.Server.AddResource(&mcp.Resource{
		Name:        "project-summary",
		Title:       "Project Summary",
		Description: "Condensed project metadata for AI agents.",
		MIMEType:    "application/json",
		URI:         s.projectSummaryURI,
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return jsonResource(s.projectSummaryURI, s.projectSummary())
	})

	s.Server.AddResource(&mcp.Resource{
		Name:        "db-schema-compact",
		Title:       "Compact DB Schema",
		Description: "Compact projection of the live database schema, keyed by table.",
		MIMEType:    "application/json",
		URI:         s.dbSchemaURI,
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return jsonResource(s.dbSchemaURI, compactSchemaDocument(s.runtime().Workspace.Schema))
	})

	s.Server.AddResource(&mcp.Resource{
		Name:        "php-symbols",
		Title:       "PHP Symbols",
		Description: "Compact top-level symbol catalog for the indexed workspace.",
		MIMEType:    "application/json",
		URI:         s.phpSymbolsURI,
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return jsonResource(s.phpSymbolsURI, s.symbolCatalog())
	})

	if s.Framework == "laravel" {
		s.Server.AddResource(&mcp.Resource{
			Name:        "laravel-routes",
			Title:       "Laravel Routes",
			Description: "Laravel route names and definition locations.",
			MIMEType:    "application/json",
			URI:         s.laravelRoutesURI,
		}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return jsonResource(s.laravelRoutesURI, s.laravelRoutes())
		})

		s.Server.AddResource(&mcp.Resource{
			Name:        "laravel-models",
			Title:       "Laravel Models",
			Description: "Indexed Laravel Eloquent models and resolved table names.",
			MIMEType:    "application/json",
			URI:         s.laravelModelsURI,
		}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return jsonResource(s.laravelModelsURI, s.laravelModels())
		})
	}

	if s.Framework == "symfony" {
		s.Server.AddResource(&mcp.Resource{
			Name:        "symfony-routes",
			Title:       "Symfony Routes",
			Description: "Symfony route names, paths, and declaration locations.",
			MIMEType:    "application/json",
			URI:         s.symfonyRoutesURI,
		}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return jsonResource(s.symfonyRoutesURI, s.symfonyRoutes())
		})
	}
}

func (s *Service) projectSummary() ProjectSummary {
	ws := s.runtime().Workspace
	var rc int
	if ws.RouteIndex != nil {
		rc = len(ws.RouteIndex.Names())
	} else if ws.Framework == "symfony" {
		rc = len(ws.SymfonyRoutes)
	}
	return ProjectSummary{
		RootPath:         s.RootPath,
		Framework:        s.Framework,
		PHPVersion:       s.BuiltinProfile.PHPVersion,
		PHPVersionSource: s.BuiltinPHPSource,
		IndexedFiles:     len(ws.Index.GetAllFileURIs()),
		RouteCount:       rc,
		SchemaSource:     schemaSource(ws.Schema),
		SchemaTables:     sortedStrings(compactSchemaTableNames(ws.Schema)),
		ContractVersion:  ContractVersion,
	}
}

func (s *Service) projectSummaryMarkdown() string {
	summary := s.projectSummary()
	var b strings.Builder
	b.WriteString("# Project Summary\n\n")
	b.WriteString("- Root: `" + summary.RootPath + "`\n")
	b.WriteString("- Framework: `" + summary.Framework + "`\n")
	b.WriteString("- PHP: `" + summary.PHPVersion + "` (" + summary.PHPVersionSource + ")\n")
	b.WriteString("- Indexed files: `" + strconv.Itoa(summary.IndexedFiles) + "`\n")
	b.WriteString("- Route count: `" + strconv.Itoa(summary.RouteCount) + "`\n")
	if len(summary.SchemaTables) > 0 {
		b.WriteString("- Schema tables: `" + strings.Join(summary.SchemaTables, "`, `") + "`\n")
	}
	if summary.SchemaSource != "" {
		b.WriteString("- Schema source: `" + summary.SchemaSource + "`\n")
	}
	return b.String()
}

func (s *Service) buildRuntime(ws *workspace.Bootstrapped) *serviceRuntime {
	rt := &serviceRuntime{Workspace: ws}

	arrayResolver := models.NewFrameworkArrayResolver(ws.Index, s.RootPath, s.Framework)
	viewResolver := frameworklaravel.NewViews(s.RootPath)
	translationResolver := frameworklaravel.NewTranslationResolver(s.RootPath)

	rt.Completion = completion.NewProvider(ws.Index, ws.Container, s.Framework)
	rt.Completion.SetArrayResolver(arrayResolver)
	if ws.RouteIndex != nil {
		rt.Completion.SetLaravelRouteIndex(ws.RouteIndex)
	}
	rt.Completion.SetViewResolver(viewResolver)
	rt.Completion.SetTranslationResolver(translationResolver)

	rt.InlayHint = inlayhint.NewProvider(ws.Index, ws.Container)
	rt.InlayHint.SetTypedChainResolver(func(expr, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
		return rt.Completion.ResolveExpressionTypeTyped(expr, source, pos, file)
	})

	rt.Hover = hover.NewProvider(ws.Index, ws.Container, s.Framework)
	rt.Hover.SetArrayResolver(arrayResolver)
	rt.Hover.SetConfig(s.Config)
	rt.Hover.GenericExprResolver = func(expr, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
		return rt.Completion.ResolveExpressionTypeTyped(expr, source, pos, file)
	}
	rt.Hover.SetTypedChainResolver(func(expr, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
		return rt.Completion.ResolveExpressionTypeTyped(expr, source, pos, file)
	})

	rt.Analyzer = analyzer.NewAnalyzer(ws.Index, ws.Container)
	if ws.RouteIndex != nil {
		rt.Analyzer.SetLaravelRouteIndex(ws.RouteIndex)
	}
	rt.Analyzer.SetChainResolver(rt.Completion.ResolveExpressionType)
	rt.Analyzer.SetTypedChainResolver(rt.Completion.ResolveExpressionTypeTyped)
	rt.Analyzer.SetViewResolver(viewResolver)
	rt.Analyzer.SetTranslationResolver(translationResolver)

	rt.Diagnostics = diagnostics.NewProvider(ws.Index, s.Framework, s.RootPath, newStdLogger(nil), s.Config)
	rt.Diagnostics.TypeResolver = func(expr, source string, line int, file *parser.FileNode) string {
		return rt.Completion.ResolveExpressionType(expr, source, protocol.Position{Line: line}, file)
	}
	rt.Diagnostics.BuilderMemberChecker = diagnostics.NewIndexMemberChecker(ws.Index)
	rt.Diagnostics.BuiltinUnavailableRule = &checks.BuiltinUnavailableRule{
		PHPVersion: s.BuiltinProfile.PHPVersion,
		Extensions: s.BuiltinProfile.Extensions,
	}
	return rt
}

func (s *Service) installRuntime(rt *serviceRuntime) {
	s.Workspace = rt.Workspace
	s.Analyzer = rt.Analyzer
	s.Completion = rt.Completion
	s.Hover = rt.Hover
	s.InlayHint = rt.InlayHint
	s.Diagnostics = rt.Diagnostics
	s.runtimeState.Store(rt)
}

func (s *Service) runtime() *serviceRuntime {
	rt := s.runtimeState.Load()
	if rt == nil {
		panic("mcp runtime not initialized")
	}
	return rt
}

func (s *Service) Reindex(ctx context.Context) (ReindexOutput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := workspace.Build(ctx, workspace.Options{
		RootPath:       s.RootPath,
		Framework:      s.Framework,
		Config:         s.Config,
		Logger:         newStdLogger(nil),
		BuiltinProfile: s.BuiltinProfile,
	})
	if err != nil {
		return ReindexOutput{}, err
	}
	s.installRuntime(s.buildRuntime(ws))
	return ReindexOutput{
		RootPath:       s.RootPath,
		Framework:      s.Framework,
		IndexedFiles:   len(ws.Index.GetAllFileURIs()),
		RouteCount:     routeCount(ws.RouteIndex),
		SchemaSource:   schemaSource(ws.Schema),
		SchemaTables:   sortedStrings(compactSchemaTableNames(ws.Schema)),
		PHPVersion:     s.BuiltinProfile.PHPVersion,
		PHPVersionFrom: s.BuiltinPHPSource,
	}, nil
}

func (s *Service) findSymbols(query string, limit int) []SymbolMatch {
	ws := s.runtime().Workspace
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 20
	}

	seen := map[string]bool{}
	var syms []*symbols.Symbol
	appendUnique := func(items []*symbols.Symbol) {
		for _, sym := range items {
			if sym == nil || seen[sym.FQN] {
				continue
			}
			seen[sym.FQN] = true
			syms = append(syms, sym)
		}
	}

	appendUnique(ws.Index.LookupByName(query))
	appendUnique(ws.Index.SearchByPrefix(query))
	fqnMatches, _ := ws.Index.SearchByFQNPrefix(query)
	appendUnique(fqnMatches)

	sort.Slice(syms, func(i, j int) bool {
		if syms[i].Name != syms[j].Name {
			return syms[i].Name < syms[j].Name
		}
		return syms[i].FQN < syms[j].FQN
	})
	if len(syms) > limit {
		syms = syms[:limit]
	}

	out := make([]SymbolMatch, 0, len(syms))
	for _, sym := range syms {
		out = append(out, SymbolMatch{
			Name:   sym.Name,
			FQN:    sym.FQN,
			Kind:   symbolKindName(sym.Kind),
			URI:    sym.URI,
			Source: symbolSourceName(sym.Source),
		})
	}
	return out
}

func (s *Service) explainSymbol(sym *symbols.Symbol) SymbolExplanation {
	ws := s.runtime().Workspace
	out := SymbolExplanation{
		Symbol:     ptrSymbolMatch(toSymbolMatch(sym)),
		Type:       sym.Type,
		ReturnType: sym.ReturnType,
		DocComment: sym.DocComment,
	}

	switch sym.Kind {
	case symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
		out.InheritanceChain = ws.Index.GetInheritanceChain(sym.FQN)
		out.ImplementedInterfaces = slices.Clone(ws.Index.GetImplementedInterfaces(sym.FQN))
		out.Members = memberList(ws.Index.GetClassMembers(sym.FQN))
		out.MemberCount = len(out.Members)
		out.Descendants = symbolFQNs(ws.Index.GetDescendants(sym.FQN))
		out.Implementors = symbolFQNs(ws.Index.GetImplementors(sym.FQN))
	default:
		ns := namespaceOf(sym.FQN)
		if ns != "" {
			out.NamespaceMembers = compactSymbols(ws.Index.GetNamespaceMembers(ns), 20)
		}
	}

	return out
}

func (s *Service) findReferences(fqn string) ([]ReferenceLocation, error) {
	rt := s.runtime()

	sym, ok := symquery.DeclarationByFQN(rt.Workspace.Index, fqn)
	if !ok {
		if rt.Workspace.Index.Lookup(fqn) == nil {
			return nil, fmt.Errorf("symbol %q not found", fqn)
		}
		return nil, fmt.Errorf("symbol %q has no source URI", fqn)
	}
	if !s.isPathReadable(symbols.URIToPath(sym.URI)) {
		return nil, fmt.Errorf("symbol %q is outside MCP allowed roots or inside denied paths", fqn)
	}

	readSource := func(uri string) (string, bool) {
		if !s.isPathReadable(symbols.URIToPath(uri)) {
			return "", false
		}
		if src := rt.Workspace.Index.GetFileSource(uri); src != "" {
			return src, true
		}
		data, err := os.ReadFile(symbols.URIToPath(uri))
		if err != nil {
			return "", false
		}
		return string(data), true
	}

	locs, ok := symquery.ReferencesByFQN(rt.Workspace.Index, rt.Analyzer, fqn, readSource)
	if !ok {
		return nil, fmt.Errorf("could not read source for symbol %q", fqn)
	}
	out := make([]ReferenceLocation, 0, len(locs))
	for _, loc := range locs {
		out = append(out, ReferenceLocation{URI: loc.URI, Range: loc.Range})
	}
	return out, nil
}

func (s *Service) projectDiagnostics(path string) (DiagnosticsOutput, error) {
	rt := s.runtime()
	if rt.Diagnostics == nil {
		return DiagnosticsOutput{}, nil
	}

	targets, err := s.diagnosticTargets(path)
	if err != nil {
		return DiagnosticsOutput{}, err
	}
	out := DiagnosticsOutput{Files: make([]FileDiagnostics, 0, len(targets))}
	for _, relPath := range targets {
		absPath := relPath
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(s.RootPath, relPath)
		}
		if !strings.HasSuffix(strings.ToLower(absPath), ".php") {
			continue
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		uri := "file://" + absPath
		diags := rt.Diagnostics.Analyze(uri, string(data))
		out.Files = append(out.Files, FileDiagnostics{
			Path:        normalizedRelativePath(s.RootPath, absPath),
			Diagnostics: diags,
		})
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	return out, nil
}

func (s *Service) diagnosticTargets(path string) ([]string, error) {
	if strings.TrimSpace(path) != "" {
		absPath := s.resolveDiagnosticPath(path)
		if !s.isPathReadable(absPath) {
			return nil, fmt.Errorf("path %q is outside MCP allowed roots or inside denied paths", path)
		}
		return []string{absPath}, nil
	}

	uris := s.runtime().Workspace.Index.GetAllFileURIs()
	seen := map[string]bool{}
	var files []string
	for _, uri := range uris {
		absPath := symbols.URIToPath(uri)
		if absPath == "" || !strings.HasPrefix(absPath, s.RootPath) || !s.isPathReadable(absPath) {
			continue
		}
		rel := normalizedRelativePath(s.RootPath, absPath)
		if rel == "" || strings.HasPrefix(rel, "vendor/") {
			continue
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		files = append(files, rel)
	}
	sort.Strings(files)
	return files, nil
}

func (s *Service) resolveDiagnosticPath(path string) string {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.RootPath, path)
}

func (s *Service) findColumn(name string) FindColumnOutput {
	schema := s.runtime().Workspace.Schema
	out := FindColumnOutput{}
	if schema == nil {
		return out
	}
	out.Connection = schema.Connection
	out.Database = schema.Database
	out.Source = schema.Source
	out.Connected = schema.Connected
	out.Caveat = schema.Caveat
	for _, table := range schema.Tables {
		for _, col := range table.Columns {
			if col.Name == name {
				out.Matches = append(out.Matches, ColumnMatch{Table: table.Name, Column: col})
			}
		}
	}
	return out
}

func (s *Service) laravelRoutes() []LaravelRouteRecord {
	ws := s.runtime().Workspace
	if ws.RouteIndex == nil {
		return nil
	}
	routes := ws.RouteIndex.All()
	out := make([]LaravelRouteRecord, 0, len(routes))
	for _, route := range routes {
		out = append(out, LaravelRouteRecord{
			Name:              route.Name,
			Path:              route.Path,
			URI:               route.URI,
			Range:             route.Range,
			TargetKind:        route.TargetKind,
			Target:            route.Target,
			HandlerFQN:        route.HandlerFQN,
			HandlerMethod:     route.HandlerMethod,
			HandlerDefinition: s.routeHandlerSymbol(&route),
			HandlerRange:      s.routeHandlerRange(&route),
		})
	}
	return out
}

func (s *Service) findLaravelRoute(name string) *LaravelRouteRecord {
	ws := s.runtime().Workspace
	if ws.RouteIndex == nil {
		return nil
	}
	route := ws.RouteIndex.Find(name)
	if route == nil {
		return nil
	}
	return &LaravelRouteRecord{
		Name:              route.Name,
		Path:              route.Path,
		URI:               route.URI,
		Range:             route.Range,
		TargetKind:        route.TargetKind,
		Target:            route.Target,
		HandlerFQN:        route.HandlerFQN,
		HandlerMethod:     route.HandlerMethod,
		HandlerDefinition: s.routeHandlerSymbol(route),
		HandlerRange:      s.routeHandlerRange(route),
	}
}

func (s *Service) symfonyRoutes() []SymfonyRouteRecord {
	ws := s.runtime().Workspace
	routes := ws.SymfonyRoutes
	out := make([]SymfonyRouteRecord, 0, len(routes))
	for _, r := range routes {
		out = append(out, SymfonyRouteRecord{
			Name:      r.Name,
			Path:      r.Path,
			URI:       r.URI,
			DeclRange: r.DeclRange,
		})
	}
	return out
}

func (s *Service) findSymfonyRoute(name string) *SymfonyRouteRecord {
	ws := s.runtime().Workspace
	for _, r := range ws.SymfonyRoutes {
		if r.Name == name {
			rec := SymfonyRouteRecord{
				Name:      r.Name,
				Path:      r.Path,
				URI:       r.URI,
				DeclRange: r.DeclRange,
			}
			return &rec
		}
	}
	return nil
}


func (s *Service) laravelModelSchema(fqn string) (LaravelModelSchemaOutput, error) {
	ws := s.runtime().Workspace
	sym := ws.Index.Lookup(fqn)
	if sym == nil {
		return LaravelModelSchemaOutput{}, fmt.Errorf("model %q not found", fqn)
	}
	tableName := models.ResolveModelTableName(ws.Index, sym, s.RootPath)
	out := LaravelModelSchemaOutput{
		Model:       ptrSymbolMatch(toSymbolMatch(sym)),
		TableName:   tableName,
		Members:     memberList(ws.Index.GetClassMembers(sym.FQN)),
		MemberCount: len(ws.Index.GetClassMembers(sym.FQN)),
	}
	if ws.Schema != nil {
		out.Table = ws.Schema.Table(tableName)
	}
	allRelations := models.ModelRelations(ws.Index, s.RootPath, s.Framework)
	for _, rel := range allRelations {
		if rel.SourceModel != fqn {
			continue
		}
		out.Relations = append(out.Relations, ModelRelationRecord{
			Method:      rel.MethodName,
			Kind:        rel.RelationType,
			Target:      rel.TargetModel,
			Cardinality: rel.Cardinality,
		})
	}
	return out, nil
}

func (s *Service) phpContainerBindings(class string) PhpContainerBindingsOutput {
	ws := s.runtime().Workspace
	out := PhpContainerBindingsOutput{
		Bindings: []ContainerBindingRecord{},
	}
	if ws.Container == nil {
		return out
	}
	rawBindings := ws.Container.GetBindings()
	keys := make([]string, 0, len(rawBindings))
	for k := range rawBindings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, abstract := range keys {
		b := rawBindings[abstract]
		out.Bindings = append(out.Bindings, ContainerBindingRecord{
			Abstract:      b.Abstract,
			Concrete:      b.Concrete,
			Singleton:     b.Singleton,
			Source:        b.Source,
			Alias:         b.Alias,
			Tags:          b.Tags,
			DefinitionURI: b.DefinitionURI,
			Range:         b.Definition,
		})
	}
	if class != "" {
		deps := ws.Container.AnalyzeConstructorInjection(class)
		out.Injection = make([]ConstructorInjectionRecord, 0, len(deps))
		for _, dep := range deps {
			out.Injection = append(out.Injection, ConstructorInjectionRecord{
				ParamName:        strings.TrimPrefix(dep.ParamName, "$"),
				TypeHint:         dep.TypeHint,
				ResolvedConcrete: dep.ResolvedConcrete,
				IsSingleton:      dep.IsSingleton,
				AutowireKind:     dep.AutowireKind,
				AutowireValue:    dep.AutowireValue,
			})
		}
	}
	return out
}

func (s *Service) symbolCatalog() []SymbolMatch {
	all := s.runtime().Workspace.Index.SearchByPrefix("")
	sort.Slice(all, func(i, j int) bool {
		if all[i].FQN != all[j].FQN {
			return all[i].FQN < all[j].FQN
		}
		return all[i].Name < all[j].Name
	})
	out := make([]SymbolMatch, 0, len(all))
	for _, sym := range all {
		if sym.Kind == symbols.KindMethod || sym.Kind == symbols.KindProperty || sym.Kind == symbols.KindConstant || sym.Kind == symbols.KindEnumCase {
			continue
		}
		out = append(out, toSymbolMatch(sym))
	}
	return out
}

func (s *Service) laravelModels() []map[string]any {
	ws := s.runtime().Workspace
	modelsList := ws.Index.GetDescendants("Illuminate\\Database\\Eloquent\\Model")
	sort.Slice(modelsList, func(i, j int) bool { return modelsList[i].FQN < modelsList[j].FQN })
	out := make([]map[string]any, 0, len(modelsList))
	for _, model := range modelsList {
		record := map[string]any{
			"symbol":    toSymbolMatch(model),
			"tableName": models.ResolveModelTableName(ws.Index, model, s.RootPath),
		}
		out = append(out, record)
	}
	return out
}

func loadProjectMetadata(rootPath string, logger *slog.Logger) (*config.Config, string, symbols.BuiltinProfile, string, error) {
	cfg := config.DefaultConfig()
	cfgPath := filepath.Join(rootPath, ".tusk-php.json")
	loaded, err := config.LoadFromFile(cfgPath)
	if err != nil {
		return nil, "", symbols.BuiltinProfile{}, "", err
	}
	cfg = loaded

	framework := cfg.Framework
	if framework == "" || framework == "auto" {
		framework = config.DetectFramework(rootPath)
	}

	timeout := time.Duration(cfg.PHPDetectTimeoutMs) * time.Millisecond
	profile, source := workspace.ResolveBuiltinProfile(rootPath, cfg.PHPBinary, timeout, func(msg string) {
		if logger != nil {
			logger.Info(msg)
		}
	})

	return cfg, framework, profile, source, nil
}

func jsonResource(uri string, value any) (*mcp.ReadResourceResult, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(body),
		}},
	}, nil
}

func compactSchema(schema *models.Schema) map[string]map[string]string {
	if schema == nil {
		return map[string]map[string]string{}
	}
	out := make(map[string]map[string]string, len(schema.Tables))
	for _, table := range schema.Tables {
		cols := make(map[string]string, len(table.Columns))
		for _, col := range table.Columns {
			typeName := col.DataType
			if col.IsNullable {
				typeName += "?"
			}
			cols[col.Name] = typeName
		}
		out[table.Name] = cols
	}
	return out
}

func compactSchemaDocument(schema *models.Schema) CompactSchemaDocument {
	doc := CompactSchemaDocument{
		Source: schemaSource(schema),
		Tables: compactSchema(schema),
	}
	if schema != nil {
		doc.Connection = schema.Connection
		doc.Database = schema.Database
		doc.Connected = schema.Connected
		doc.Caveat = schema.Caveat
	}
	return doc
}

func compactSchemaTableNames(schema *models.Schema) []string {
	if schema == nil {
		return nil
	}
	return schema.TableNames()
}

func routeCount(index *frameworklaravel.RouteIndex) int {
	if index == nil {
		return 0
	}
	return len(index.Names())
}

func schemaSource(schema *models.Schema) string {
	if schema == nil || schema.Source == "" {
		return models.SchemaSourceNone
	}
	return schema.Source
}

func sortedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := slices.Clone(items)
	sort.Strings(out)
	return out
}

func symbolKindName(kind symbols.SymbolKind) string {
	switch kind {
	case symbols.KindClass:
		return "class"
	case symbols.KindInterface:
		return "interface"
	case symbols.KindTrait:
		return "trait"
	case symbols.KindEnum:
		return "enum"
	case symbols.KindFunction:
		return "function"
	case symbols.KindMethod:
		return "method"
	case symbols.KindProperty:
		return "property"
	case symbols.KindConstant:
		return "constant"
	case symbols.KindVariable:
		return "variable"
	case symbols.KindEnumCase:
		return "enum_case"
	case symbols.KindNamespace:
		return "namespace"
	default:
		return "unknown"
	}
}

func toSymbolMatch(sym *symbols.Symbol) SymbolMatch {
	return SymbolMatch{
		Name:   sym.Name,
		FQN:    sym.FQN,
		Kind:   symbolKindName(sym.Kind),
		URI:    sym.URI,
		Source: symbolSourceName(sym.Source),
	}
}

func ptrSymbolMatch(sym SymbolMatch) *SymbolMatch {
	return &sym
}

func memberList(members []*symbols.Symbol) []SymbolMember {
	out := make([]SymbolMember, 0, len(members))
	for _, member := range members {
		out = append(out, SymbolMember{
			Name:       member.Name,
			FQN:        member.FQN,
			Kind:       symbolKindName(member.Kind),
			Type:       member.Type,
			ReturnType: member.ReturnType,
			Virtual:    member.IsVirtual,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FQN < out[j].FQN })
	return out
}

func symbolFQNs(items []*symbols.Symbol) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.FQN)
	}
	sort.Strings(out)
	return out
}

func compactSymbols(items []*symbols.Symbol, limit int) []SymbolMatch {
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]SymbolMatch, 0, len(items))
	for _, item := range items {
		out = append(out, toSymbolMatch(item))
	}
	return out
}

func namespaceOf(fqn string) string {
	if idx := strings.LastIndex(fqn, "\\"); idx >= 0 {
		return fqn[:idx]
	}
	return ""
}

func normalizedRelativePath(rootPath, absPath string) string {
	rel, err := filepath.Rel(rootPath, absPath)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}

func (s *Service) isPathReadable(absPath string) bool {
	if absPath == "" {
		return false
	}
	cleanRoot := filepath.Clean(s.RootPath)
	cleanPath := filepath.Clean(absPath)
	relCheck, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return false
	}
	rel := normalizedRelativePath(cleanRoot, cleanPath)
	if rel == "." {
		return true
	}
	for _, deny := range s.Config.AIDenyPaths() {
		deny = filepath.ToSlash(filepath.Clean(deny))
		if rel == deny || strings.HasPrefix(rel, deny+"/") {
			return false
		}
	}
	allowed := s.Config.AIRoots()
	if len(allowed) == 0 {
		return true
	}
	for _, root := range allowed {
		root = filepath.ToSlash(filepath.Clean(root))
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}

func writeJSONFile(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeFile(path, body)
}

func writeFile(path string, body []byte) error {
	return os.WriteFile(path, body, 0644)
}

func (s *Service) routeHandlerSymbol(route *frameworklaravel.RouteName) *SymbolMatch {
	ws := s.runtime().Workspace
	if route == nil {
		return nil
	}
	if route.HandlerMethod != "" {
		if sym := ws.Index.Lookup(route.HandlerFQN + "::" + route.HandlerMethod); sym != nil {
			match := toSymbolMatch(sym)
			return &match
		}
	}
	if route.HandlerFQN != "" {
		if sym := ws.Index.Lookup(route.HandlerFQN); sym != nil {
			match := toSymbolMatch(sym)
			return &match
		}
	}
	return nil
}

func (s *Service) routeHandlerRange(route *frameworklaravel.RouteName) *protocol.Range {
	ws := s.runtime().Workspace
	if route == nil {
		return nil
	}
	if route.HandlerMethod != "" {
		if sym := ws.Index.Lookup(route.HandlerFQN + "::" + route.HandlerMethod); sym != nil {
			rng := sym.Range
			return &rng
		}
	}
	if route.HandlerFQN != "" {
		if sym := ws.Index.Lookup(route.HandlerFQN); sym != nil {
			rng := sym.Range
			return &rng
		}
	}
	return nil
}

func symbolSourceName(source symbols.SymbolSource) string {
	switch source {
	case symbols.SourceProject:
		return "project"
	case symbols.SourceBuiltin:
		return "builtin"
	case symbols.SourceVendor:
		return "vendor"
	default:
		return "unknown"
	}
}

func newStdLogger(logger *slog.Logger) *log.Logger {
	return log.New(&slogWriter{logger: logger}, "", 0)
}

type slogWriter struct {
	logger *slog.Logger
}

func (w *slogWriter) Write(p []byte) (int, error) {
	if w.logger != nil {
		w.logger.Info(strings.TrimSpace(string(p)))
	}
	return len(p), nil
}


