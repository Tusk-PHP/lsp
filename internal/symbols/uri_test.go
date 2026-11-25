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

func TestURISlashPathReadsWindowsURIs(t *testing.T) {
	for uri, want := range map[string]string{
		`file://C:\proj\app\Order.php`:    "C:/proj/app/Order.php",
		"file:///C:/proj/app/Order.php":   "C:/proj/app/Order.php",
		"file:///c%3A/proj/app/Order.php": "c:/proj/app/Order.php",
		"file:///C:/proj/my%20app/a.php":  "C:/proj/my app/a.php",
		`file://D:\proj\app\..\Order.php`: "D:/proj/Order.php",
		"file:///srv/app/Order.php":       "/srv/app/Order.php",
	} {
		if got := uriSlashPath(uri, true); got != want {
			t.Errorf("%s: got %s, want %s", uri, got, want)
		}
	}
	if got := uriSlashPath(`file://C:\proj\a.php`, false); got == "C:/proj/a.php" {
		t.Errorf("a backslash is part of a file name outside Windows: %s", got)
	}
}
