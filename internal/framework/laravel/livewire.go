package laravel

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

// livewireLocations returns the directories Livewire 4 registers as
// locations for views and anonymous components, from component_locations in
// config/livewire.php or Livewire's own default config. known is false when
// an entry can't be read, so a view or component may live anywhere.
func livewireLocations(root string) (dirs []string, known bool) {
	if !isDir(filepath.Join(root, "vendor", "livewire", "livewire")) {
		return nil, true
	}
	data, err := os.ReadFile(filepath.Join(root, "config", "livewire.php"))
	if err != nil {
		data, err = os.ReadFile(filepath.Join(root, "vendor", "livewire", "livewire", "config", "livewire.php"))
		if err != nil {
			return nil, true
		}
	}
	src := string(data)
	config := phpparser.Parse(src, token.Latest).ReturnedArray()
	if config == nil {
		return nil, false
	}
	for _, item := range config.Items {
		if item == nil {
			continue
		}
		if key, ok := ast.StringValue(item.Key, src); ok && key == "component_locations" {
			return livewireLocationDirs(root, item.Value, src)
		}
	}
	return nil, true // Livewire 3 registers no locations.
}

func livewireLocationDirs(root string, value ast.Expr, src string) ([]string, bool) {
	locations, ok := value.(*ast.Array)
	if !ok {
		return nil, false
	}
	dirs := make([]string, 0, len(locations.Items))
	for _, item := range locations.Items {
		dir, ok := livewireLocationDir(root, item, src)
		if !ok {
			return nil, false
		}
		dirs = append(dirs, dir)
	}
	return dirs, true
}

func livewireLocationDir(root string, item *ast.ArrayItem, src string) (string, bool) {
	if item == nil {
		return "", false
	}
	call, ok := item.Value.(*ast.Call)
	if !ok || item.Key != nil || item.Unpack || call.Args == nil || len(call.Args.Args) != 1 {
		return "", false
	}
	callee, ok := call.Callee.(*ast.ConstFetch)
	if !ok {
		return "", false
	}
	arg := call.Args.Args[0]
	if arg.Name != nil || arg.Unpack {
		return "", false
	}
	rel, ok := ast.StringValue(arg.Value, src)
	if !ok {
		return "", false
	}
	var base string
	switch strings.ToLower(strings.TrimPrefix(callee.Name.Text(src), `\`)) {
	case "resource_path":
		base = filepath.Join(root, "resources")
	case "app_path":
		base = filepath.Join(root, "app")
	case "base_path":
		base = root
	default:
		return "", false
	}
	return filepath.Join(base, filepath.FromSlash(rel)), true
}
