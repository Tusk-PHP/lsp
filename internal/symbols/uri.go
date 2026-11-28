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
	localHost   = "localhost"
	uncPrefix   = "//"
	uncShare    = "UNC/"
	deviceHost  = "."
	extendHost  = "?"
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
	return pathToURI(path, hostWindows)
}

func pathToURI(path string, windows bool) string {
	p, err := slashPath(path, windows)
	if err != nil {
		return path
	}
	return slashPathToURI(p)
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
		return fileURIPath(uri, rest, windows)
	}
	u, err := url.Parse(uri)
	switch {
	case err != nil:
		return "", fmt.Errorf("%q is not a file URI: %w", uri, err)
	case u.Scheme != "file":
		return "", fmt.Errorf("%q is not a file URI", uri)
	case u.Path == "":
		return "", fmt.Errorf("%q is not a file URI: it names no path", uri)
	case u.User != nil:
		return "", fmt.Errorf("%q is not a file URI: it names a user", uri)
	}
	host := strings.ToLower(u.Host)
	switch {
	case host == "" || host == localHost:
		return fileURIPath(uri, u.Path, windows)
	case !windows:
		return "", fmt.Errorf("%q is not a local file URI: it names the host %q", uri, u.Host)
	case !isUNCHost(host):
		return "", fmt.Errorf("%q is not a file URI: %q is not a host a UNC path can name", uri, u.Host)
	}
	return uncSlashPath(host, u.Path), nil
}

func fileURIPath(uri, p string, windows bool) (string, error) {
	slashed, err := slashPath(p, windows)
	if err != nil {
		return "", fmt.Errorf("%q is not a file URI: %w", uri, err)
	}
	return slashed, nil
}

func slashPath(p string, windows bool) (string, error) {
	if !windows {
		return path.Clean(p), nil
	}
	p, err := win32Path(p)
	if err != nil {
		return "", err
	}
	if host, rest, ok := splitUNC(p); ok {
		return uncSlashPath(host, rest), nil
	}
	p = path.Clean(p)
	if strings.HasPrefix(p, "/") && startsWithDrive(p[1:]) {
		p = p[1:]
	}
	if startsWithDrive(p) {
		p = strings.ToUpper(p[:1]) + p[1:]
	}
	return p, nil
}

func win32Path(p string) (string, error) {
	slashed := strings.ReplaceAll(p, `\`, "/")
	after, ok := strings.CutPrefix(slashed, uncPrefix)
	if !ok {
		return slashed, nil
	}
	namespace, rest, _ := strings.Cut(after, "/")
	switch namespace {
	case deviceHost:
		return "", fmt.Errorf("%q is a device path, not a file", p)
	case extendHost:
		return extendedLengthPath(p, rest)
	}
	return slashed, nil
}

func extendedLengthPath(p, rest string) (string, error) {
	if len(rest) >= len(uncShare) && strings.EqualFold(rest[:len(uncShare)], uncShare) {
		unc := uncPrefix + rest[len(uncShare):]
		if _, _, ok := splitUNC(unc); !ok {
			return "", fmt.Errorf("%q is an extended-length path that names no share", p)
		}
		return unc, nil
	}
	if !startsWithDrive(rest) {
		return "", fmt.Errorf("%q is an extended-length path that names no drive or share", p)
	}
	return rest, nil
}

func slashPathToURI(p string) string {
	if host, rest, ok := splitUNC(p); ok && host != localHost {
		return fileURI(host, rest)
	}
	if !strings.HasPrefix(p, "/") {
		p = path.Clean("/" + p)
	}
	return fileURI("", p)
}

func fileURI(host, p string) string {
	return (&url.URL{Scheme: "file", Host: host, Path: p}).String()
}

func slashDir(p string) string {
	if host, rest, ok := splitUNC(p); ok {
		return uncPrefix + host + path.Dir(rest)
	}
	return path.Dir(p)
}

func splitUNC(p string) (host, rest string, ok bool) {
	after, ok := strings.CutPrefix(p, uncPrefix)
	if !ok {
		return "", "", false
	}
	host, rest, _ = strings.Cut(after, "/")
	host = strings.ToLower(host)
	return host, "/" + rest, isUNCHost(host)
}

func uncSlashPath(host, p string) string {
	return uncPrefix + host + path.Clean(strings.ReplaceAll(p, `\`, "/"))
}

func isUNCHost(host string) bool {
	if host == "" || host == deviceHost {
		return false
	}
	for i := 0; i < len(host); i++ {
		c := host[i]
		if c < 0x80 && !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || strings.IndexByte("-._~!$&'()*+,;=", c) >= 0) {
			return false
		}
	}
	return true
}

func startsWithDrive(p string) bool {
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	letter := p[0] | 0x20
	return 'a' <= letter && letter <= 'z'
}
