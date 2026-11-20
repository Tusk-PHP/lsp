package composer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SuggestedNamespaces guesses the namespaces of the packages the project's
// composer.json lists under "suggest" that aren't installed. A library refers
// to such optional packages in code that only runs once the user installs
// them (Laravel's Ably and Pusher broadcasters), so their classes are missing
// by design.
//
// The guesses are lowercase prefixes ending in `\`, taken from the package
// name: the first word of the name (ably/ably-php → `ably\`,
// pusher/pusher-php-server → `pusher\`) and the vendor followed by the name
// (intervention/image → `intervention\image\`). A vendor alone is never a
// guess, so symfony/mercure doesn't cover every `Symfony\` class. Platform
// packages (php, ext-*) are skipped: extensions are handled by the builtin
// profile. The result is sorted; nil when there is nothing to suggest.
func SuggestedNamespaces(rootPath string) []string {
	data, err := os.ReadFile(filepath.Join(rootPath, "composer.json"))
	if err != nil {
		return nil
	}
	var cj composerJSON
	if json.Unmarshal(data, &cj) != nil || len(cj.Suggest) == 0 {
		return nil
	}
	vendorDir := filepath.Join(rootPath, "vendor")
	seen := map[string]bool{}
	var out []string
	add := func(prefix string) {
		if !seen[prefix] {
			seen[prefix] = true
			out = append(out, prefix)
		}
	}
	for pkg := range cj.Suggest {
		if isPlatformPackage(pkg) {
			continue
		}
		vendor, name, ok := strings.Cut(strings.ToLower(pkg), "/")
		if !ok || vendor == "" || name == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(vendorDir, vendor, name)); err == nil {
			continue // installed: its classes are indexed, so a miss is real
		}
		first, _, _ := strings.Cut(name, "-")
		if first = namespaceWord(first); len(first) >= 3 && first != "php" {
			add(first + `\`)
		}
		if v, n := namespaceWord(vendor), namespaceWord(name); v != "" && n != "" {
			add(v + `\` + n + `\`)
		}
	}
	sort.Strings(out)
	return out
}

// namespaceWord turns a package name part into the namespace segment it
// usually becomes, lowercased: `ably-php` → `ablyphp`, `fork` → `fork`.
func namespaceWord(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
		}
	}
	return b.String()
}
