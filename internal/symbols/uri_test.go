package symbols

import (
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
		"file://localhost/srv/x.php",
		"file://Server/share/../x.php",
		"file:////server/share/x.php",
		"file:////localhost/c$/x.php",
		"file://user@host/x.php",
		"file://host:445/x.php",
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
		if local, ok := strings.CutPrefix(uri, "file:///"); ok {
			if got := canonicalURI("file://localhost/"+local, windows); got != canonical {
				t.Errorf("%q (windows %v): with localhost as its host its canonical is %q, not %q", uri, windows, got, canonical)
			}
		}
		if u, err := url.Parse(uri); err == nil && u.Host != "" && !strings.EqualFold(u.Host, localHost) && !(windows && startsWithDrive(strings.TrimPrefix(uri, fileScheme))) {
			if c, err := url.Parse(canonical); err != nil || c.Host != strings.ToLower(u.Host) {
				t.Errorf("%q (windows %v): its canonical %q does not keep the host %q", uri, windows, canonical, u.Host)
			}
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

func TestAFileURIWithAHostIsAUNCPathUnderWindowsRules(t *testing.T) {
	for uri, want := range map[string]struct{ path, canonical string }{
		"file://server/share/app/Foo.php":         {"//server/share/app/Foo.php", "file://server/share/app/Foo.php"},
		"file://SERVER/share/app/../Foo.php":      {"//server/share/Foo.php", "file://server/share/Foo.php"},
		"file://server/share/my%20app/caf%C3%A9":  {"//server/share/my app/café", "file://server/share/my%20app/caf%C3%A9"},
		"file:////server/share/Foo.php":           {"//server/share/Foo.php", "file://server/share/Foo.php"},
		"file://server/share\\app\\Foo.php":       {"//server/share/app/Foo.php", "file://server/share/app/Foo.php"},
		"file://localhost//localhost/c$/Foo.php":  {"//localhost/c$/Foo.php", "file:////localhost/c$/Foo.php"},
		"file://caf%C3%A9.example/share/Foo.php":  {"//café.example/share/Foo.php", "file://caf%C3%A9.example/share/Foo.php"},
		"file:////c:/proj/Foo.php":                {"C:/proj/Foo.php", "file:///C:/proj/Foo.php"},
		"file://fe80--1.ipv6-literal.net/s/a.php": {"//fe80--1.ipv6-literal.net/s/a.php", "file://fe80--1.ipv6-literal.net/s/a.php"},
	} {
		if got, err := uriSlashPath(uri, true); err != nil || got != want.path {
			t.Errorf("%s: got %s, %v, want %s", uri, got, err, want.path)
		}
		if got := canonicalURI(uri, true); got != want.canonical {
			t.Errorf("%s: canonical %s, want %s", uri, got, want.canonical)
		}
	}
	for _, uri := range []string{"file://user@server/share/Foo.php", "file://server:445/share/Foo.php", "file://[::1]/share/Foo.php", "file://server:/share/Foo.php"} {
		if got, err := uriSlashPath(uri, true); err == nil {
			t.Errorf("%s names no UNC path, got %s", uri, got)
		}
		if got := canonicalURI(uri, true); got != uri {
			t.Errorf("%s is kept as the client spelled it, got %s", uri, got)
		}
	}
}

func TestPathToURIWritesAUNCPathWithItsHost(t *testing.T) {
	for path, want := range map[string]string{
		`\\server\share\app\Foo.php`:   "file://server/share/app/Foo.php",
		`\\Server\share\my app\#1.php`: "file://server/share/my%20app/%231.php",
		`//server/share/app/Foo.php`:   "file://server/share/app/Foo.php",
		`\\server`:                     "file://server/",
		`\\localhost\c$\Foo.php`:       "file:////localhost/c$/Foo.php",
		`\\\srv\Foo.php`:               "file:///srv/Foo.php",
		`\\c:\proj\Foo.php`:            "file:///C:/proj/Foo.php",
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

func TestLocalhostIsTheLocalMachine(t *testing.T) {
	for _, windows := range []bool{false, true} {
		for uri, want := range map[string]string{
			"file://localhost/srv/app/Foo.php": "file:///srv/app/Foo.php",
			"file://LocalHost/srv/app/Foo.php": "file:///srv/app/Foo.php",
		} {
			if got := canonicalURI(uri, windows); got != want {
				t.Errorf("%s (windows %v): got %s, want %s", uri, windows, got, want)
			}
		}
	}
	if got := canonicalURI("file://localhost/c%3A/proj/Foo.php", true); got != "file:///C:/proj/Foo.php" {
		t.Errorf("a localhost URI with a drive names the drive: %s", got)
	}
}

func TestAFileURIWithAHostIsNoLocalFileUnderPOSIXRules(t *testing.T) {
	uri := "file://server/share/app/Foo.php"
	if got := canonicalURI(uri, false); got != uri {
		t.Errorf("a URI naming another host is kept as the client spelled it, got %s", got)
	}
	if got, err := uriSlashPath(uri, false); err == nil || !strings.Contains(err.Error(), `"server"`) {
		t.Errorf("the error names the host: %s, %v", got, err)
	}
}

func TestFileTreeListsUNCFilesUnderAShare(t *testing.T) {
	tree := fileTree{windows: true}
	uris := []string{"file://server/share/app/A.php", "file://server/share/app/sub/B.php", "file://server/other/C.php"}
	for _, uri := range uris {
		tree.add(uri)
	}
	got := tree.under(`\\server\share`)
	slices.Sort(got)
	if want := uris[:2]; !slices.Equal(got, want) {
		t.Errorf("files under \\\\server\\share: got %v, want %v", got, want)
	}
	if got := tree.under(`\\SERVER`); len(got) != 3 {
		t.Errorf("every share of the host sits under it: %v", got)
	}
	for _, uri := range uris {
		tree.remove(uri)
	}
	if got := tree.under(`\\server`); len(got) != 0 {
		t.Errorf("removed files leave the host: %v", got)
	}
}
