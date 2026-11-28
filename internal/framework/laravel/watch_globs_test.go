package laravel

import (
	"path"
	"strings"
	"testing"
)

func TestWatchGlobsCoverTheJSONTranslationFiles(t *testing.T) {
	for _, file := range []string{"lang/de.json", "resources/lang/de.json"} {
		matched := false
		for _, glob := range WatchGlobs() {
			tail := strings.TrimPrefix(glob, "**/")
			for _, suffix := range []string{file, file[strings.Index(file, "lang/"):]} {
				if ok, err := path.Match(tail, suffix); err == nil && ok {
					matched = true
				}
			}
		}
		if !matched {
			t.Errorf("no watch glob in %q covers %s", WatchGlobs(), file)
		}
	}
}
