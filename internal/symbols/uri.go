package symbols

import (
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	fileScheme  = "file://"
	hostWindows = runtime.GOOS == "windows"
)

func URIToPath(uri string) string {
	return filepath.FromSlash(uriSlashPath(uri, hostWindows))
}

func uriSlashPath(uri string, windows bool) string {
	if rest, ok := strings.CutPrefix(uri, fileScheme); ok && windows && startsWithDrive(rest) {
		return slashPath(rest, windows)
	}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return slashPath(uri, windows)
	}
	p := u.Path
	if windows && strings.HasPrefix(p, "/") && startsWithDrive(p[1:]) {
		p = p[1:]
	}
	return path.Clean(p)
}

func slashPath(p string, windows bool) string {
	if windows {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	return path.Clean(p)
}

func startsWithDrive(p string) bool {
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	letter := p[0] | 0x20
	return 'a' <= letter && letter <= 'z'
}

func PathToURI(path string) string {
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String()
}
