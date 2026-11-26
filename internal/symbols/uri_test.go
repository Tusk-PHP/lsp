package symbols

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		if back, err := URIToPath(got); err != nil || back != filepath.Clean("/"+trimSlash(path)) {
			t.Errorf("%s does not round-trip: %s, %v", got, back, err)
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
		if got, err := uriSlashPath(uri, true); err != nil || got != want {
			t.Errorf("%s: got %s, %v, want %s", uri, got, err, want)
		}
	}
	if got, err := uriSlashPath("file:///c:/proj/x.php", false); err != nil || got != "/c:/proj/x.php" {
		t.Errorf("outside Windows a drive-like segment is an ordinary directory: %s, %v", got, err)
	}
	if got, err := uriSlashPath(`file://C:\proj\a.php`, false); err == nil {
		t.Errorf("outside Windows a backslash path in the authority is no file URI: %s", got)
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
		if back, err := uriSlashPath(uri, true); err != nil || back != slashPath(path, true) {
			t.Errorf("%s does not round-trip: %s, %v", uri, back, err)
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

func TestCanonicalURIIsTheURIPathToURIBuilds(t *testing.T) {
	built := PathToURI(filepath.Join(t.TempDir(), "team@shop", "Order.php"))
	escaped := strings.Replace(built, "team@shop", "team%40shop", 1)
	if escaped == built {
		t.Fatalf("PathToURI leaves @ unescaped: %s", built)
	}
	for _, uri := range []string{built, escaped} {
		if got := CanonicalURI(uri); got != built {
			t.Errorf("CanonicalURI(%s) = %s, want %s", uri, got, built)
		}
	}
}

func TestAURIThatNamesNoReadableFileIsNotAFileURI(t *testing.T) {
	for _, uri := range []string{"file:///a%zz", "file:", "file:Order.php", "file://host", "untitled:Untitled-1", "builtin", "/already/a/path", ""} {
		for _, windows := range []bool{false, true} {
			if got := canonicalURI(uri, windows); got != uri {
				t.Errorf("%q (windows %v) is kept as the client spelled it, got %q", uri, windows, got)
			}
		}
		if path, err := URIToPath(uri); err == nil || path != "" || !strings.Contains(err.Error(), strconv.Quote(uri)) {
			t.Errorf("%q has no path: got %q, %v", uri, path, err)
		}
		if _, err := ReadURI(uri); err == nil {
			t.Errorf("%q is not read from disk", uri)
		}
	}
	if _, err := URIToPath("file:///a%zz"); !strings.Contains(err.Error(), "invalid URL escape") {
		t.Errorf("the error says why the URI does not parse: %v", err)
	}
}

func TestReadURIReadsTheFileAFileURINames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my app", "Order.php")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, err := ReadURI(PathToURI(path)); err != nil || string(data) != "<?php\n" {
		t.Errorf("got %q, %v", data, err)
	}
}

func FuzzCanonicalFileURIIsThePathToURIOfItsPath(f *testing.F) {
	for _, uri := range []string{
		"file:///srv/app/Order.php",
		"file:///c%3A/proj/x.php",
		"file:///C:/proj/my%20app/a%40b.php",
		"file:Order.php",
		"file:",
		"file:/../a%3A",
		"file:///srv/../../x",
		"file://host/share/x.php",
		"file:///a%zz",
		`file://C:\proj\a.php`,
		"untitled:Untitled-1",
	} {
		f.Add(uri, false)
		f.Add(uri, true)
	}
	f.Fuzz(func(t *testing.T, uri string, windows bool) {
		canonical := canonicalURI(uri, windows)
		if _, err := uriSlashPath(uri, windows); err != nil {
			if canonical != uri {
				t.Errorf("%q (windows %v) is not a file URI (%v), but is respelled %q", uri, windows, err, canonical)
			}
			return
		}
		path, err := uriSlashPath(canonical, windows)
		if err != nil {
			t.Fatalf("%q (windows %v): its canonical %q is not a file URI: %v", uri, windows, canonical, err)
		}
		if rebuilt := slashPathToURI(slashPath(path, windows)); rebuilt != canonical {
			t.Errorf("%s (windows %v): canonical %s, but the URI of its path is %s", uri, windows, canonical, rebuilt)
		}
	})
}
