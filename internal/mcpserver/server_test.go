package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testdataPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(file), "..", "..", "testdata")
	all := append([]string{base}, parts...)
	return filepath.Join(all...)
}

var (
	sharedLaravelOnce sync.Once
	sharedLaravelSvc_ *Service
	sharedLaravelErr  error

	sharedSymfonyOnce sync.Once
	sharedSymfonySvc_ *Service
	sharedSymfonyErr  error
)

func sharedLaravelSvc(t *testing.T) *Service {
	t.Helper()
	sharedLaravelOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "laravel")
		sharedLaravelSvc_, sharedLaravelErr = New(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	})
	if sharedLaravelErr != nil {
		t.Fatalf("shared laravel workspace: %v", sharedLaravelErr)
	}
	return sharedLaravelSvc_
}

func sharedSymfonySvc(t *testing.T) *Service {
	t.Helper()
	sharedSymfonyOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "symfony")
		sharedSymfonySvc_, sharedSymfonyErr = New(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	})
	if sharedSymfonyErr != nil {
		t.Fatalf("shared symfony workspace: %v", sharedSymfonyErr)
	}
	return sharedSymfonySvc_
}

func TestServiceToolsAndResources(t *testing.T) {
	ctx := context.Background()
	svc := sharedLaravelSvc(t)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	ct, st := mcp.NewInMemoryTransports()
	serverSession, err := svc.Server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	defer serverSession.Wait()

	clientSession, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("Client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	var toolNames []string
	for _, tool := range tools.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	if !slices.Contains(toolNames, "php_project_summary") || !slices.Contains(toolNames, "php_find_symbol") || !slices.Contains(toolNames, "db_describe_table") {
		t.Fatalf("unexpected tools: %v", toolNames)
	}
	if !slices.Contains(toolNames, "php_explain_symbol") || !slices.Contains(toolNames, "php_find_references") || !slices.Contains(toolNames, "php_diagnostics") || !slices.Contains(toolNames, "laravel_routes") || !slices.Contains(toolNames, "laravel_model_schema") {
		t.Fatalf("expected extended MCP tool surface, got %v", toolNames)
	}

	summary, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "php_project_summary"})
	if err != nil {
		t.Fatalf("CallTool(project_summary) error = %v", err)
	}
	summaryMap := structuredMap(t, summary.StructuredContent)
	if got := summaryMap["framework"]; got != "laravel" {
		t.Fatalf("expected laravel framework, got %#v", got)
	}

	findRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "php_find_symbol",
		Arguments: map[string]any{"query": "Product"},
	})
	if err != nil {
		t.Fatalf("CallTool(find_symbol) error = %v", err)
	}
	findMap := structuredMap(t, findRes.StructuredContent)
	results, ok := findMap["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatalf("expected non-empty symbol results, got %#v", findMap["results"])
	}

	explainRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "php_explain_symbol",
		Arguments: map[string]any{"fqn": `App\Models\Product`},
	})
	if err != nil {
		t.Fatalf("CallTool(explain_symbol) error = %v", err)
	}
	explainMap := structuredMap(t, explainRes.StructuredContent)
	if explainMap["memberCount"] == nil {
		t.Fatalf("expected symbol explanation payload, got %#v", explainMap)
	}

	refsRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "php_find_references",
		Arguments: map[string]any{"fqn": `App\Models\Product`},
	})
	if err != nil {
		t.Fatalf("CallTool(find_references) error = %v", err)
	}
	refsMap := structuredMap(t, refsRes.StructuredContent)
	if refsMap["references"] == nil {
		t.Fatalf("expected references payload, got %#v", refsMap)
	}

	diagRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "php_diagnostics",
		Arguments: map[string]any{"path": "app/Http/Controllers/CategoryController.php"},
	})
	if err != nil {
		t.Fatalf("CallTool(php_diagnostics) error = %v", err)
	}
	diagMap := structuredMap(t, diagRes.StructuredContent)
	if diagMap["files"] == nil {
		t.Fatalf("expected diagnostics payload, got %#v", diagMap)
	}

	resources, err := clientSession.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources() error = %v", err)
	}
	var uris []string
	for _, resource := range resources.Resources {
		uris = append(uris, resource.URI)
	}
	if !slices.Contains(uris, "tusk://project/summary") || !slices.Contains(uris, "tusk://db/schema/compact") || !slices.Contains(uris, "tusk://php/symbols") || !slices.Contains(uris, "tusk://laravel/routes") || !slices.Contains(uris, "tusk://laravel/models") {
		t.Fatalf("unexpected resource URIs: %v", uris)
	}

	resource, err := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: "tusk://project/summary"})
	if err != nil {
		t.Fatalf("ReadResource() error = %v", err)
	}
	if len(resource.Contents) != 1 || !json.Valid([]byte(resource.Contents[0].Text)) {
		t.Fatalf("expected JSON project summary resource, got %#v", resource.Contents)
	}
}

func TestDescribeTableTool(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "app.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL, bio TEXT)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DB_CONNECTION=sqlite\nDB_DATABASE="+dbPath+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(`{"framework":"laravel","databaseEnabled":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"autoload":{"psr-4":{"App\\\\":"app/"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(context.Background(), root, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	ct, st := mcp.NewInMemoryTransports()
	serverSession, err := svc.Server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	defer serverSession.Wait()

	clientSession, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("Client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "db_describe_table",
		Arguments: map[string]any{"name": "users"},
	})
	if err != nil {
		t.Fatalf("CallTool(db_describe_table) error = %v", err)
	}
	out := structuredMap(t, result.StructuredContent)
	table, ok := out["table"].(map[string]any)
	if !ok {
		t.Fatalf("expected table object, got %#v", out["table"])
	}
	if got := table["name"]; got != "users" {
		t.Fatalf("expected users table, got %#v", got)
	}
	if got := out["source"]; got != "live" {
		t.Fatalf("expected live source, got %#v", got)
	}
}

func TestLaravelToolsAndDBList(t *testing.T) {
	ctx := context.Background()
	svc := sharedLaravelSvc(t)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	ct, st := mcp.NewInMemoryTransports()
	serverSession, err := svc.Server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	defer serverSession.Wait()

	clientSession, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("Client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	routesRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "laravel_routes"})
	if err != nil {
		t.Fatalf("CallTool(laravel_routes) error = %v", err)
	}
	raw, err := json.Marshal(routesRes.StructuredContent)
	if err != nil {
		t.Fatalf("json.Marshal(routes) error = %v", err)
	}
	var routesOut map[string][]map[string]any
	if err := json.Unmarshal(raw, &routesOut); err != nil {
		t.Fatalf("json.Unmarshal(routes) error = %v", err)
	}
	if len(routesOut["routes"]) == 0 {
		t.Fatal("expected laravel routes")
	}

	modelRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "laravel_model_schema",
		Arguments: map[string]any{"fqn": `App\Models\Product`},
	})
	if err != nil {
		t.Fatalf("CallTool(laravel_model_schema) error = %v", err)
	}
	modelMap := structuredMap(t, modelRes.StructuredContent)
	if got := modelMap["tableName"]; got != "products" {
		t.Fatalf("expected products table, got %#v", got)
	}

	listTablesRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "db_list_tables"})
	if err != nil {
		t.Fatalf("CallTool(db_list_tables) error = %v", err)
	}
	listTablesMap := structuredMap(t, listTablesRes.StructuredContent)
	if listTablesMap["tables"] == nil {
		t.Fatalf("expected db_list_tables payload, got %#v", listTablesMap)
	}
}

func TestLaravelRouteToControllerTool(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app", "Http", "Controllers"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "app", "Providers"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "routes"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(`{"framework":"laravel","databaseEnabled":false}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"autoload":{"psr-4":{"App\\\\":"app/"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "artisan"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "Providers", "AppServiceProvider.php"), []byte("<?php\nnamespace App\\Providers;\nclass AppServiceProvider {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	controllerSource := `<?php
namespace App\Http\Controllers;

class HomeController
{
    public function index()
    {
    }
}
`
	if err := os.WriteFile(filepath.Join(root, "app", "Http", "Controllers", "HomeController.php"), []byte(controllerSource), 0644); err != nil {
		t.Fatal(err)
	}
	routeSource := `<?php
use App\Http\Controllers\HomeController;
use Illuminate\Support\Facades\Route;

Route::get('/', [HomeController::class, 'index'])->name('home');
`
	if err := os.WriteFile(filepath.Join(root, "routes", "web.php"), []byte(routeSource), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(context.Background(), root, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	ct, st := mcp.NewInMemoryTransports()
	serverSession, err := svc.Server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	defer serverSession.Wait()

	clientSession, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("Client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "laravel_route_to_controller",
		Arguments: map[string]any{"name": "home"},
	})
	if err != nil {
		t.Fatalf("CallTool(laravel_route_to_controller) error = %v", err)
	}
	out := structuredMap(t, result.StructuredContent)
	if got := out["handlerFqn"]; got != `App\Http\Controllers\HomeController` {
		t.Fatalf("expected HomeController handler, got %#v", got)
	}
	if got := out["handlerMethod"]; got != "index" {
		t.Fatalf("expected index handler method, got %#v", got)
	}
	handlerDef, ok := out["handlerDefinition"].(map[string]any)
	if !ok || handlerDef["fqn"] != `App\Http\Controllers\HomeController::index` {
		t.Fatalf("expected resolved handler definition, got %#v", out["handlerDefinition"])
	}
}

func TestDiagnosticsToolFindsIssues(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(`{"framework":"","databaseEnabled":false}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"autoload":{"psr-4":{"App\\\\":"src/"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	source := `<?php
namespace App;
use DateTime;

class Example {
    public function run(): void {
        return;
        $x = 1;
    }
}
`
	if err := os.WriteFile(filepath.Join(root, "src", "Example.php"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(context.Background(), root, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	ct, st := mcp.NewInMemoryTransports()
	serverSession, err := svc.Server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	defer serverSession.Wait()

	clientSession, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("Client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "php_diagnostics",
		Arguments: map[string]any{"path": "src/Example.php"},
	})
	if err != nil {
		t.Fatalf("CallTool(php_diagnostics) error = %v", err)
	}

	out := structuredMap(t, result.StructuredContent)
	files, ok := out["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("expected one diagnostics file entry, got %#v", out["files"])
	}
	file, ok := files[0].(map[string]any)
	if !ok {
		t.Fatalf("expected diagnostics file object, got %#v", files[0])
	}
	diags, ok := file["diagnostics"].([]any)
	if !ok || len(diags) == 0 {
		t.Fatalf("expected diagnostics, got %#v", file["diagnostics"])
	}
}

func TestDiagnosticsToolRespectsAISafetyPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "vendor", "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := `{
  "framework": "",
  "databaseEnabled": false,
  "ai": {
    "allowedRoots": ["app"],
    "denyPaths": ["vendor"]
  }
}`
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"autoload":{"psr-4":{"App\\\\":"app/"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	appSource := `<?php
namespace App;
class Example { public function run(): void { return; $x = 1; } }
`
	if err := os.WriteFile(filepath.Join(root, "app", "Example.php"), []byte(appSource), 0644); err != nil {
		t.Fatal(err)
	}
	vendorSource := `<?php
namespace Vendor;
class Hidden {}
`
	if err := os.WriteFile(filepath.Join(root, "vendor", "pkg", "Hidden.php"), []byte(vendorSource), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(context.Background(), root, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	ct, st := mcp.NewInMemoryTransports()
	serverSession, err := svc.Server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	defer serverSession.Wait()

	clientSession, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("Client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "php_diagnostics",
	})
	if err != nil {
		t.Fatalf("CallTool(php_diagnostics) error = %v", err)
	}
	out := structuredMap(t, result.StructuredContent)
	files, ok := out["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("expected one visible diagnostics file, got %#v", out["files"])
	}

	denied, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "php_diagnostics",
		Arguments: map[string]any{"path": "vendor/pkg/Hidden.php"},
	})
	if err != nil {
		t.Fatalf("CallTool(php_diagnostics denied path) error = %v", err)
	}
	if !denied.IsError {
		t.Fatal("expected denied-path diagnostics result to be marked as error")
	}
}

func TestDumpWritesContextPackFiles(t *testing.T) {
	ctx := context.Background()
	svc := sharedLaravelSvc(t)

	outDir := filepath.Join(t.TempDir(), "ai-context")
	if err := svc.Dump(ctx, outDir); err != nil {
		t.Fatalf("Dump() error = %v", err)
	}

	wantFiles := []string{
		"project-summary.md",
		"db-schema.compact.json",
		"symbols.json",
		"laravel-routes.json",
		"models.json",
	}
	for _, name := range wantFiles {
		path := filepath.Join(outDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected dump file %s: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("expected non-empty dump file %s", name)
		}
		if strings.HasSuffix(name, ".json") && !json.Valid(data) {
			t.Fatalf("expected valid JSON in %s", name)
		}
	}
}

func TestMigrationSchemaProvenanceInMCP(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app", "Providers"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "database", "migrations"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(`{"framework":"laravel","databaseEnabled":true,"databaseSource":"migrations"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"autoload":{"psr-4":{"App\\\\":"app/"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "artisan"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "Providers", "AppServiceProvider.php"), []byte("<?php\nnamespace App\\Providers;\nclass AppServiceProvider {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	migration := `<?php
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;
Schema::create('users', function (Blueprint $table) {
    $table->id();
    $table->string('email');
    $table->timestamps();
});
`
	if err := os.WriteFile(filepath.Join(root, "database", "migrations", "2024_01_01_000000_create_users_table.php"), []byte(migration), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(context.Background(), root, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	doc := compactSchemaDocument(svc.Workspace.Schema)
	if doc.Source != "migrations" {
		t.Fatalf("expected migrations source, got %q", doc.Source)
	}
	if doc.Connected {
		t.Fatal("expected migrations schema to be disconnected")
	}
	if doc.Caveat == "" {
		t.Fatal("expected caveat for migrations schema")
	}
	if _, ok := doc.Tables["users"]; !ok {
		t.Fatal("expected users table in migration schema document")
	}
}

func TestReindexToolRebuildsWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(`{"framework":"","databaseEnabled":false}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"autoload":{"psr-4":{"App\\\\":"app/"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "Example.php"), []byte("<?php\nnamespace App;\nclass Example {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(context.Background(), root, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	ct, st := mcp.NewInMemoryTransports()
	serverSession, err := svc.Server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	defer serverSession.Wait()

	clientSession, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("Client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	before, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "php_find_symbol",
		Arguments: map[string]any{"query": "Added"},
	})
	if err != nil {
		t.Fatalf("CallTool(find_symbol before reindex) error = %v", err)
	}
	beforeMap := structuredMap(t, before.StructuredContent)
	if results, _ := beforeMap["results"].([]any); len(results) != 0 {
		t.Fatalf("expected no Added symbol before reindex, got %#v", beforeMap["results"])
	}

	if err := os.WriteFile(filepath.Join(root, "app", "Added.php"), []byte("<?php\nnamespace App;\nclass Added {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	reindexRes, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "tusk_reindex"})
	if err != nil {
		t.Fatalf("CallTool(tusk_reindex) error = %v", err)
	}
	reindexMap := structuredMap(t, reindexRes.StructuredContent)
	if reindexRes.IsError {
		t.Fatalf("expected successful reindex result, got error payload %#v", reindexRes.StructuredContent)
	}
	if got, ok := reindexMap["indexedFiles"].(float64); !ok || got < 2 {
		t.Fatalf("expected indexedFiles >= 2 after reindex, got %#v", reindexMap["indexedFiles"])
	}

	after, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "php_find_symbol",
		Arguments: map[string]any{"query": "Added"},
	})
	if err != nil {
		t.Fatalf("CallTool(find_symbol after reindex) error = %v", err)
	}
	afterMap := structuredMap(t, after.StructuredContent)
	results, ok := afterMap["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatalf("expected Added symbol after reindex, got %#v", afterMap["results"])
	}
}

func TestConcurrentReadsRemainStableDuringReindex(t *testing.T) {
	// Use a minimal temp project so that each Reindex call is fast (no
	// large vendor tree to walk).  The full laravel testdata fixture has
	// ~9 600 vendor PHP files, which makes three reindex cycles exceed the
	// CI test timeout when race detection is enabled.
	root := t.TempDir()
	for _, dir := range []string{
		filepath.Join(root, "app", "Models"),
		filepath.Join(root, "app", "Http", "Controllers"),
		filepath.Join(root, "app", "Providers"),
		filepath.Join(root, "routes"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "artisan"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"autoload":{"psr-4":{"App\\":"app/"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(`{"framework":"laravel","databaseEnabled":false}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "Providers", "AppServiceProvider.php"), []byte("<?php\nnamespace App\\Providers;\nclass AppServiceProvider {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "Models", "Product.php"), []byte("<?php\nnamespace App\\Models;\nclass Product {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "routes", "web.php"), []byte("<?php\nuse Illuminate\\Support\\Facades\\Route;\nRoute::get('/', fn() => null);\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, root, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	probeFile := filepath.Join(root, "app", "Http", "Controllers", "ReindexProbe.php")

	var wg sync.WaitGroup
	errCh := make(chan error, 16)

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = svc.projectSummary()
				if got := svc.findSymbols("Product", 5); len(got) == 0 {
					errCh <- io.ErrUnexpectedEOF
					return
				}
				_ = svc.laravelRoutes()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 3; j++ {
			body := "<?php\nnamespace App\\Http\\Controllers;\nclass ReindexProbe" + strings.Repeat("X", j) + " {}\n"
			if err := os.WriteFile(probeFile, []byte(body), 0644); err != nil {
				errCh <- err
				return
			}
			if _, err := svc.Reindex(ctx); err != nil {
				errCh <- err
				return
			}
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent read/reindex error = %v", err)
		}
	}
}

func structuredMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(structured) error = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("json.Unmarshal(structured) error = %v", err)
	}
	return out
}

// newInProcessClient sets up an in-process MCP client/server pair and returns
// the client session (and a cleanup function). Callers must defer the cleanup.
func newInProcessClient(t *testing.T, svc *Service) (*mcp.ClientSession, func()) {
	t.Helper()
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	ct, st := mcp.NewInMemoryTransports()
	serverSession, err := svc.Server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("Server.Connect() error = %v", err)
	}
	clientSession, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("Client.Connect() error = %v", err)
	}
	return clientSession, func() {
		clientSession.Close()
		serverSession.Wait()
	}
}

func TestSymfonyToolsVisible(t *testing.T) {
	ctx := context.Background()
	svc := sharedSymfonySvc(t)
	if svc.Framework != "symfony" {
		t.Fatalf("expected symfony framework, got %q", svc.Framework)
	}

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	var toolNames []string
	for _, tool := range tools.Tools {
		toolNames = append(toolNames, tool.Name)
	}

	// Symfony-specific tools must be present.
	if !slices.Contains(toolNames, "symfony_routes") {
		t.Errorf("expected symfony_routes tool, got tools: %v", toolNames)
	}
	if !slices.Contains(toolNames, "symfony_route_to_controller") {
		t.Errorf("expected symfony_route_to_controller tool, got tools: %v", toolNames)
	}
	// php_container_bindings is always registered regardless of framework.
	if !slices.Contains(toolNames, "php_container_bindings") {
		t.Errorf("expected php_container_bindings tool, got tools: %v", toolNames)
	}
	// Also verify that a Laravel workspace does NOT get symfony_* tools.
	laravelClient, laravelCleanup := newInProcessClient(t, sharedLaravelSvc(t))
	defer laravelCleanup()

	laravelTools, err := laravelClient.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools(laravel) error = %v", err)
	}
	var laravelToolNames []string
	for _, tool := range laravelTools.Tools {
		laravelToolNames = append(laravelToolNames, tool.Name)
	}
	if slices.Contains(laravelToolNames, "symfony_routes") {
		t.Errorf("did not expect symfony_routes in laravel workspace, tools: %v", laravelToolNames)
	}
	if slices.Contains(laravelToolNames, "symfony_route_to_controller") {
		t.Errorf("did not expect symfony_route_to_controller in laravel workspace, tools: %v", laravelToolNames)
	}
}

func TestLaravelToolsVisible(t *testing.T) {
	ctx := context.Background()
	svc := sharedLaravelSvc(t)
	if svc.Framework != "laravel" {
		t.Fatalf("expected laravel framework, got %q", svc.Framework)
	}

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	var toolNames []string
	for _, tool := range tools.Tools {
		toolNames = append(toolNames, tool.Name)
	}

	// Laravel-specific tools must be present for a Laravel workspace.
	if !slices.Contains(toolNames, "laravel_routes") {
		t.Errorf("expected laravel_routes tool, got tools: %v", toolNames)
	}
	if !slices.Contains(toolNames, "laravel_route_to_controller") {
		t.Errorf("expected laravel_route_to_controller tool, got tools: %v", toolNames)
	}
	if !slices.Contains(toolNames, "laravel_model_schema") {
		t.Errorf("expected laravel_model_schema tool, got tools: %v", toolNames)
	}
	// php_graph must always be present.
	if !slices.Contains(toolNames, "php_graph") {
		t.Errorf("expected php_graph tool, got tools: %v", toolNames)
	}

	// Also verify that a Symfony workspace does NOT get laravel_* tools.
	symfonyClient, symfonyCleanup := newInProcessClient(t, sharedSymfonySvc(t))
	defer symfonyCleanup()

	symfonyTools, err := symfonyClient.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools(symfony) error = %v", err)
	}
	var symfonyToolNames []string
	for _, tool := range symfonyTools.Tools {
		symfonyToolNames = append(symfonyToolNames, tool.Name)
	}
	if slices.Contains(symfonyToolNames, "laravel_routes") {
		t.Errorf("did not expect laravel_routes in symfony workspace, tools: %v", symfonyToolNames)
	}
	if slices.Contains(symfonyToolNames, "laravel_route_to_controller") {
		t.Errorf("did not expect laravel_route_to_controller in symfony workspace, tools: %v", symfonyToolNames)
	}
	if slices.Contains(symfonyToolNames, "laravel_model_schema") {
		t.Errorf("did not expect laravel_model_schema in symfony workspace, tools: %v", symfonyToolNames)
	}
}

func TestSymfonyRoutesResource(t *testing.T) {
	ctx := context.Background()
	svc := sharedSymfonySvc(t)

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	resources, err := clientSession.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources() error = %v", err)
	}
	var uris []string
	for _, r := range resources.Resources {
		uris = append(uris, r.URI)
	}
	if !slices.Contains(uris, "tusk://symfony/routes") {
		t.Errorf("expected tusk://symfony/routes resource, got: %v", uris)
	}
	if slices.Contains(uris, "tusk://laravel/routes") {
		t.Errorf("did not expect tusk://laravel/routes resource in symfony workspace")
	}

	res, err := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: "tusk://symfony/routes"})
	if err != nil {
		t.Fatalf("ReadResource(symfony-routes) error = %v", err)
	}
	if len(res.Contents) != 1 || !json.Valid([]byte(res.Contents[0].Text)) {
		t.Fatalf("expected valid JSON symfony-routes resource, got %#v", res.Contents)
	}
}

func TestSymfonyRouteTools(t *testing.T) {
	ctx := context.Background()
	svc := sharedSymfonySvc(t)

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	// symfony_routes should return a list with at least one route from ProductController.
	routesRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "symfony_routes"})
	if err != nil {
		t.Fatalf("CallTool(symfony_routes) error = %v", err)
	}
	routesMap := structuredMap(t, routesRes.StructuredContent)
	routes, ok := routesMap["routes"].([]any)
	if !ok || len(routes) == 0 {
		t.Fatalf("expected non-empty symfony routes, got %#v", routesMap["routes"])
	}
	// At least one route should have a name.
	firstRoute, ok := routes[0].(map[string]any)
	if !ok || firstRoute["name"] == "" {
		t.Fatalf("expected route with name, got %#v", routes[0])
	}

	// symfony_route_to_controller: look up a known route (product_index).
	lookupRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "symfony_route_to_controller",
		Arguments: map[string]any{"name": "product_index"},
	})
	if err != nil {
		t.Fatalf("CallTool(symfony_route_to_controller) error = %v", err)
	}
	if lookupRes.IsError {
		t.Fatalf("expected successful route lookup, got error: %#v", lookupRes.StructuredContent)
	}
	lookupMap := structuredMap(t, lookupRes.StructuredContent)
	if got := lookupMap["name"]; got != "product_index" {
		t.Fatalf("expected product_index route, got %#v", got)
	}

	// symfony_route_to_controller: unknown route should return error.
	notFoundRes, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "symfony_route_to_controller",
		Arguments: map[string]any{"name": "no_such_route"},
	})
	if err != nil {
		t.Fatalf("CallTool(symfony_route_to_controller not-found) error = %v", err)
	}
	if !notFoundRes.IsError {
		t.Fatal("expected error result for unknown route")
	}
}

func TestProjectSummaryContractVersion(t *testing.T) {
	ctx := context.Background()
	svc := sharedLaravelSvc(t)

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "php_project_summary"})
	if err != nil {
		t.Fatalf("CallTool(php_project_summary) error = %v", err)
	}
	m := structuredMap(t, res.StructuredContent)
	cv, ok := m["contractVersion"].(float64)
	if !ok || int(cv) != 1 {
		t.Errorf("expected contractVersion=1 in project summary, got %#v", m["contractVersion"])
	}
}


func TestPhpContainerBindings(t *testing.T) {
	ctx := context.Background()
	svc := sharedLaravelSvc(t)

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	// Without class: expect default Laravel bindings to be returned.
	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "php_container_bindings"})
	if err != nil {
		t.Fatalf("CallTool(php_container_bindings) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("expected successful result, got error: %#v", res.StructuredContent)
	}
	out := structuredMap(t, res.StructuredContent)
	bindings, ok := out["bindings"].([]any)
	if !ok || len(bindings) == 0 {
		t.Fatalf("expected non-empty bindings, got %#v", out["bindings"])
	}
	// Verify at least one binding has abstract and concrete fields.
	first, ok := bindings[0].(map[string]any)
	if !ok {
		t.Fatalf("expected binding to be a map, got %T", bindings[0])
	}
	if first["abstract"] == "" {
		t.Errorf("expected non-empty abstract in first binding, got %#v", first)
	}
	if first["concrete"] == "" {
		t.Errorf("expected non-empty concrete in first binding, got %#v", first)
	}
	// Verify a known default binding is present (e.g. abstract=cache).
	found := false
	for _, raw := range bindings {
		b, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if b["abstract"] == "cache" {
			found = true
			if b["concrete"] != "Illuminate\\Cache\\CacheManager" {
				t.Errorf("expected cache binding concrete=Illuminate\\Cache\\CacheManager, got %#v", b["concrete"])
			}
			if singleton, ok := b["singleton"].(bool); !ok || !singleton {
				t.Errorf("expected cache binding to be singleton, got %#v", b["singleton"])
			}
			break
		}
	}
	if !found {
		t.Errorf("expected cache binding in laravel container bindings, got %v bindings", len(bindings))
	}
	// Verify injection is absent when class is not specified.
	if _, present := out["injection"]; present {
		t.Errorf("expected injection to be absent when class is not specified, got %#v", out["injection"])
	}
}

func TestPhpContainerBindingsWithSymfony(t *testing.T) {
	ctx := context.Background()
	svc := sharedSymfonySvc(t)

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	// php_container_bindings must be reachable from a Symfony workspace too.
	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "php_container_bindings"})
	if err != nil {
		t.Fatalf("CallTool(php_container_bindings) symfony error = %v", err)
	}
	if res.IsError {
		t.Fatalf("expected successful result for symfony, got error: %#v", res.StructuredContent)
	}
	out := structuredMap(t, res.StructuredContent)
	if _, ok := out["bindings"]; !ok {
		t.Fatalf("expected bindings key in output, got %#v", out)
	}
}

func TestPhpContainerBindingsWithClass(t *testing.T) {
	ctx := context.Background()
	svc := sharedLaravelSvc(t)

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	// Call php_container_bindings with class=App\Services\Checkout.
	// App\Services\Checkout has a constructor that injects PaymentGateway,
	// which is bound to StripeGateway via $this->app->singleton(...) in AppServiceProvider.
	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "php_container_bindings",
		Arguments: map[string]any{"class": `App\Services\Checkout`},
	})
	if err != nil {
		t.Fatalf("CallTool(php_container_bindings with class) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("expected successful result, got error: %#v", res.StructuredContent)
	}
	out := structuredMap(t, res.StructuredContent)

	// Verify that the PaymentGateway binding is present in the bindings list.
	bindings, ok := out["bindings"].([]any)
	if !ok || len(bindings) == 0 {
		t.Fatalf("expected non-empty bindings, got %#v", out["bindings"])
	}
	foundBinding := false
	for _, raw := range bindings {
		b, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if b["abstract"] == `App\Services\PaymentGateway` {
			foundBinding = true
			if b["concrete"] != `App\Services\StripeGateway` {
				t.Errorf("expected concrete=App\\Services\\StripeGateway, got %#v", b["concrete"])
			}
			if singleton, ok := b["singleton"].(bool); !ok || !singleton {
				t.Errorf("expected PaymentGateway binding to be singleton, got %#v", b["singleton"])
			}
			break
		}
	}
	if !foundBinding {
		t.Errorf("expected App\\Services\\PaymentGateway binding in bindings list, got %d bindings", len(bindings))
	}

	// Verify injection is populated and resolves the gateway parameter.
	injection, ok := out["injection"].([]any)
	if !ok || len(injection) == 0 {
		t.Fatalf("expected non-empty injection for App\\Services\\Checkout, got %#v", out["injection"])
	}
	foundParam := false
	for _, raw := range injection {
		dep, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if dep["paramName"] == "gateway" {
			foundParam = true
			if dep["typeHint"] != `App\Services\PaymentGateway` {
				t.Errorf("expected typeHint=App\\Services\\PaymentGateway, got %#v", dep["typeHint"])
			}
			if dep["resolvedConcrete"] != `App\Services\StripeGateway` {
				t.Errorf("expected resolvedConcrete=App\\Services\\StripeGateway, got %#v", dep["resolvedConcrete"])
			}
			if singleton, ok := dep["isSingleton"].(bool); !ok || !singleton {
				t.Errorf("expected isSingleton=true for gateway param, got %#v", dep["isSingleton"])
			}
			break
		}
	}
	if !foundParam {
		t.Errorf("expected gateway parameter in injection, got %#v", injection)
	}
}

func TestLaravelModelSchemaRelations(t *testing.T) {
	ctx := context.Background()
	svc := sharedLaravelSvc(t)

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	// App\Models\Category has a products() hasMany relation to App\Models\Product.
	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "laravel_model_schema",
		Arguments: map[string]any{"fqn": `App\Models\Category`},
	})
	if err != nil {
		t.Fatalf("CallTool(laravel_model_schema) error = %v", err)
	}
	if res.IsError {
		t.Fatalf("expected successful result, got error: %#v", res.StructuredContent)
	}
	out := structuredMap(t, res.StructuredContent)
	relations, ok := out["relations"].([]any)
	if !ok || len(relations) == 0 {
		t.Fatalf("expected relations for App\\Models\\Category, got %#v", out["relations"])
	}
	// Verify the products() hasMany relation to App\Models\Product is present.
	found := false
	for _, raw := range relations {
		rel, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if rel["method"] == "products" {
			found = true
			if rel["kind"] != "HasMany" {
				t.Errorf("expected HasMany kind, got %#v", rel["kind"])
			}
			if rel["target"] != `App\Models\Product` {
				t.Errorf("expected target=App\\Models\\Product, got %#v", rel["target"])
			}
			if rel["cardinality"] != "many" {
				t.Errorf("expected cardinality=many, got %#v", rel["cardinality"])
			}
			break
		}
	}
	if !found {
		t.Errorf("expected products() relation in App\\Models\\Category relations, got %#v", relations)
	}
}


func TestProjectSummaryContractVersion(t *testing.T) {
	ctx := context.Background()
	svc := sharedLaravelSvc(t)

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "php_project_summary"})
	if err != nil {
		t.Fatalf("CallTool(php_project_summary) error = %v", err)
	}
	m := structuredMap(t, res.StructuredContent)
	cv, ok := m["contractVersion"].(float64)
	if !ok || int(cv) != 1 {
		t.Errorf("expected contractVersion=1 in project summary, got %#v", m["contractVersion"])
	}
}

