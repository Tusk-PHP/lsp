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

func TestServiceToolsAndResources(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, testdataPath(t, "laravel"), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, testdataPath(t, "laravel"), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, testdataPath(t, "laravel"), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, testdataPath(t, "symfony"), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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
	// php_graph must always be present.
	if !slices.Contains(toolNames, "php_graph") {
		t.Errorf("expected php_graph tool, got tools: %v", toolNames)
	}

	// Also verify that a Laravel workspace does NOT get symfony_* tools.
	laravelSvc, err := New(ctx, testdataPath(t, "laravel"), logger)
	if err != nil {
		t.Fatalf("New(laravel) error = %v", err)
	}
	laravelClient, laravelCleanup := newInProcessClient(t, laravelSvc)
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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, testdataPath(t, "laravel"), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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
	symfonySvc, err := New(ctx, testdataPath(t, "symfony"), logger)
	if err != nil {
		t.Fatalf("New(symfony) error = %v", err)
	}
	symfonyClient, symfonyCleanup := newInProcessClient(t, symfonySvc)
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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, testdataPath(t, "symfony"), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, testdataPath(t, "symfony"), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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

func TestPhpGraphTool(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, testdataPath(t, "laravel"), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	for _, kind := range []string{"container", "references", "models"} {
		t.Run("json_"+kind, func(t *testing.T) {
			res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
				Name:      "php_graph",
				Arguments: map[string]any{"kind": kind, "format": "json"},
			})
			if err != nil {
				t.Fatalf("CallTool(php_graph kind=%s) error = %v", kind, err)
			}
			if res.IsError {
				t.Fatalf("expected success, got error payload: %#v", res.StructuredContent)
			}
			m := structuredMap(t, res.StructuredContent)
			// Graph DTO must carry schemaVersion == 1 and matching kind.
			if v, _ := m["schemaVersion"].(float64); int(v) != 1 {
				t.Errorf("expected schemaVersion=1, got %v", m["schemaVersion"])
			}
			if m["kind"] != kind {
				t.Errorf("expected kind=%s, got %v", kind, m["kind"])
			}
		})
	}

	t.Run("mermaid_container", func(t *testing.T) {
		res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
			Name:      "php_graph",
			Arguments: map[string]any{"kind": "container", "format": "mermaid"},
		})
		if err != nil {
			t.Fatalf("CallTool(php_graph mermaid) error = %v", err)
		}
		if res.IsError {
			t.Fatalf("expected success for mermaid format: %#v", res.StructuredContent)
		}
		m := structuredMap(t, res.StructuredContent)
		if m["format"] != "mermaid" {
			t.Errorf("expected format=mermaid, got %v", m["format"])
		}
		if text, _ := m["text"].(string); !strings.HasPrefix(text, "graph LR") {
			t.Errorf("expected mermaid graph LR header, got %q", text)
		}
	})

	t.Run("dot_references", func(t *testing.T) {
		res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
			Name:      "php_graph",
			Arguments: map[string]any{"kind": "references", "format": "dot"},
		})
		if err != nil {
			t.Fatalf("CallTool(php_graph dot) error = %v", err)
		}
		if res.IsError {
			t.Fatalf("expected success for dot format: %#v", res.StructuredContent)
		}
		m := structuredMap(t, res.StructuredContent)
		if m["format"] != "dot" {
			t.Errorf("expected format=dot, got %v", m["format"])
		}
		if text, _ := m["text"].(string); !strings.HasPrefix(text, "digraph") {
			t.Errorf("expected digraph header, got %q", text)
		}
	})
}

func TestPhpGraphToolInvalidInput(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, testdataPath(t, "laravel"), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	clientSession, cleanup := newInProcessClient(t, svc)
	defer cleanup()

	cases := []struct {
		name string
		args map[string]any
	}{
		{name: "bad_kind", args: map[string]any{"kind": "bogus"}},
		{name: "bad_deps", args: map[string]any{"kind": "container", "deps": "bogus"}},
		{name: "bad_format", args: map[string]any{"kind": "container", "format": "bogus"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
				Name:      "php_graph",
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool(php_graph %s) unexpected transport error: %v", tc.name, err)
			}
			if !res.IsError {
				t.Errorf("expected error result for invalid input %v, got %#v", tc.args, res.StructuredContent)
			}
		})
	}
}

func TestProjectSummaryContractVersion(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(ctx, testdataPath(t, "laravel"), logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
