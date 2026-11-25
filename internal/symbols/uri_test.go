package symbols

import (
	"path/filepath"
	"testing"
)

func TestPathToURIEncodesWhatAURIPathCannotHold(t *testing.T) {
	for path, want := range map[string]string{
		"/srv/my app/index.php":   "file:///srv/my%20app/index.php",
		"/srv/a#b%c.php":          "file:///srv/a%23b%25c.php",
		"/srv/café/Ünïcode.php":   "file:///srv/caf%C3%A9/%C3%9Cn%C3%AFcode.php",
		"/srv/100% #1 é/x.php":    "file:///srv/100%25%20%231%20%C3%A9/x.php",
		"relative/x.php":          "file:///relative/x.php",
		"/srv/app/../lib/./x.php": "file:///srv/lib/x.php",
	} {
		got := PathToURI(path)
		if got != want {
			t.Errorf("%s: got %s, want %s", path, got, want)
		}
		if back := URIToPath(got); back != filepath.Clean("/"+trimSlash(path)) {
			t.Errorf("%s does not round-trip: %s", got, back)
		}
	}
}

func TestURIToPathReadsWindowsURIsWithOneDriveLetterCase(t *testing.T) {
	for uri, want := range map[string]string{
		`file://C:\proj\app\Order.php`:        "C:/proj/app/Order.php",
		`file://d:\proj\app\..\Order.php`:     "D:/proj/Order.php",
		"file:///C:/proj/app/Order.php":       "C:/proj/app/Order.php",
		"file:///c%3A/proj/app/Order.php":     "C:/proj/app/Order.php",
		"file:///c:/proj/my%20app/a%23b.php":  "C:/proj/my app/a#b.php",
		"file:///d:/proj/caf%C3%A9/Order.php": "D:/proj/café/Order.php",
		"file:///srv/app/Order.php":           "/srv/app/Order.php",
	} {
		if got := uriSlashPath(uri, true); got != want {
			t.Errorf("%s: got %s, want %s", uri, got, want)
		}
	}
	if got := uriSlashPath("file:///c:/proj/x.php", false); got != "/c:/proj/x.php" {
		t.Errorf("outside Windows a drive-like segment is an ordinary directory: %s", got)
	}
	if got := uriSlashPath(`file://C:\proj\a.php`, false); got == "C:/proj/a.php" {
		t.Errorf("a backslash is part of a file name outside Windows: %s", got)
	}
}

func TestPathToURIWritesWindowsPathsWithOneDriveLetterCase(t *testing.T) {
	for path, want := range map[string]string{
		`c:\proj\app\Order.php`: "file:///C:/proj/app/Order.php",
		`C:\proj\my app\#1.php`: "file:///C:/proj/my%20app/%231.php",
		"d:/proj/café.php":      "file:///D:/proj/caf%C3%A9.php",
	} {
		uri := slashPathToURI(slashPath(path, true))
		if uri != want {
			t.Errorf("%s: got %s, want %s", path, uri, want)
		}
		if back := uriSlashPath(uri, true); back != slashPath(path, true) {
			t.Errorf("%s does not round-trip: %s", uri, back)
		}
	}
}

func trimSlash(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}

func TestCanonicalURISpellsEveryClientsURIForAFileTheSameWay(t *testing.T) {
	for uri, want := range map[string]string{
		"file:///c%3A/proj/app/Order.php":    "file:///C:/proj/app/Order.php",
		"file:///c:/proj/app/Order.php":      "file:///C:/proj/app/Order.php",
		"file:///C:/proj/my%20app/a%40b.php": "file:///C:/proj/my%20app/a@b.php",
		"file:///C:/proj/caf%c3%a9.php":      "file:///C:/proj/caf%C3%A9.php",
		"untitled:Untitled-1":                "untitled:Untitled-1",
	} {
		if got := canonicalURI(uri, true); got != want {
			t.Errorf("%s: got %s, want %s", uri, got, want)
		}
	}
	for uri, want := range map[string]string{
		"file:///srv/my%20app/a%40b.php":   "file:///srv/my%20app/a@b.php",
		"file:///srv/app/./x/../Order.php": "file:///srv/app/Order.php",
		"builtin://php/strlen":             "builtin://php/strlen",
	} {
		if got := canonicalURI(uri, false); got != want {
			t.Errorf("%s: got %s, want %s", uri, got, want)
		}
	}
}
