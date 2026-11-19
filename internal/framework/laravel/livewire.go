package laravel

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// livewireLocationCall is one entry of Livewire's component_locations:
// resource_path('views/livewire'), base_path('…') or app_path('…').
var livewireLocationCall = regexp.MustCompile(`^(resource_path|base_path|app_path)\(\s*['"]([^'"$]*)['"]\s*\)$`)

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
	cfg := string(data)
	k := strings.Index(cfg, "'component_locations'")
	if k < 0 {
		return nil, true // Livewire 3 registers no locations.
	}
	rest := cfg[k:]
	open := strings.IndexByte(rest, '[')
	end := strings.IndexByte(rest, ']')
	if open < 0 || end < open {
		return nil, false
	}
	for _, entry := range strings.Split(rest[open+1:end], ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" || strings.HasPrefix(entry, "//") {
			continue
		}
		m := livewireLocationCall.FindStringSubmatch(entry)
		if m == nil {
			return nil, false
		}
		base := root
		switch m[1] {
		case "resource_path":
			base = filepath.Join(root, "resources")
		case "app_path":
			base = filepath.Join(root, "app")
		}
		dirs = append(dirs, filepath.Join(base, filepath.FromSlash(m[2])))
	}
	return dirs, true
}
