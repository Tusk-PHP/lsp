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

func PathToURI(path string) string {
	return slashPathToURI(slashPath(path, hostWindows))
}

func CanonicalURI(uri string) string {
	return canonicalURI(uri, hostWindows)
}

func IsFileURI(uri string) bool {
	return strings.HasPrefix(uri, "file:")
}

func canonicalURI(uri string, windows bool) string {
	if !IsFileURI(uri) {
		return uri
	}
	return slashPathToURI(uriSlashPath(uri, windows))
}

func uriSlashPath(uri string, windows bool) string {
	if rest, ok := strings.CutPrefix(uri, fileScheme); ok && windows && startsWithDrive(rest) {
		return slashPath(rest, windows)
	}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return slashPath(uri, windows)
	}
	return slashPath(u.Path, windows)
}

func slashPath(p string, windows bool) string {
	if !windows {
		return path.Clean(p)
	}
	p = path.Clean(strings.ReplaceAll(p, `\`, "/"))
	if strings.HasPrefix(p, "/") && startsWithDrive(p[1:]) {
		p = p[1:]
	}
	if startsWithDrive(p) {
		p = strings.ToUpper(p[:1]) + p[1:]
	}
	return p
}

func slashPathToURI(p string) string {
	if !strings.HasPrefix(p, "/") {
		p = path.Clean("/" + p)
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

func startsWithDrive(p string) bool {
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	letter := p[0] | 0x20
	return 'a' <= letter && letter <= 'z'
}
