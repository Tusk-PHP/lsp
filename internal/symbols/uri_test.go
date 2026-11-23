package symbols

import "testing"

func TestPathToURI(t *testing.T) {
	for path, want := range map[string]string{
		"/srv/my app/index.php": "file:///srv/my%20app/index.php",
		"/srv/a#b%c.php":        "file:///srv/a%23b%25c.php",
		"relative/x.php":        "file:///relative/x.php",
	} {
		got := PathToURI(path)
		if got != want {
			t.Errorf("%s: got %s, want %s", path, got, want)
		}
		if back := URIToPath(got); back != "/"+trimSlash(path) {
			t.Errorf("%s does not round-trip: %s", got, back)
		}
	}
}

func trimSlash(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}
