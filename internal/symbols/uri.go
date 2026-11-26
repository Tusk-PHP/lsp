package symbols

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	fileScheme  = "file://"
	hostWindows = runtime.GOOS == "windows"
)

func URIToPath(uri string) (string, error) {
	p, err := uriSlashPath(uri, hostWindows)
	if err != nil {
		return "", err
	}
	return filepath.FromSlash(p), nil
}

func ReadURI(uri string) ([]byte, error) {
	path, err := URIToPath(uri)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func PathToURI(path string) string {
	return slashPathToURI(slashPath(path, hostWindows))
}

func CanonicalURI(uri string) string {
	return canonicalURI(uri, hostWindows)
}

func canonicalURI(uri string, windows bool) string {
	p, err := uriSlashPath(uri, windows)
	if err != nil {
		return uri
	}
	return slashPathToURI(p)
}

func uriSlashPath(uri string, windows bool) (string, error) {
	if rest, ok := strings.CutPrefix(uri, fileScheme); ok && windows && startsWithDrive(rest) {
		return slashPath(rest, windows), nil
	}
	u, err := url.Parse(uri)
	switch {
	case err != nil:
		return "", fmt.Errorf("%q is not a file URI: %w", uri, err)
	case u.Scheme != "file":
		return "", fmt.Errorf("%q is not a file URI", uri)
	case u.Path == "":
		return "", fmt.Errorf("%q is not a file URI: it names no path", uri)
	}
	return slashPath(u.Path, windows), nil
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
