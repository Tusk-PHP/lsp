package stubs

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed php
var phpFS embed.FS

var stubPaths = embeddedStubPaths(phpFS)

// Entry is an embedded builtin stub file ready for indexing by the parent.
type Entry struct {
	Path    string
	Content string
}

// Profile selects the builtin stub layers that should be indexed for a project.
type Profile struct {
	PHPVersion string
	Extensions []string
}

// BuiltinPHP returns embedded PHP builtin stubs in deterministic path order.
func BuiltinPHP() []Entry {
	return BuiltinPHPForProfile(DefaultProfile())
}

// DefaultProfile returns the broadest bundled profile for compatibility with
// older callers that do not yet provide Composer platform information.
func DefaultProfile() Profile {
	return Profile{
		PHPVersion: "8.6",
		Extensions: []string{
			"json",
		},
	}
}

// BuiltinPHPForProfile returns embedded PHP builtin stubs in deterministic path
// order for the selected PHP version and extension set. PHP 7.4 files are the
// baseline; later files are deltas and are included only when <= PHPVersion.
func BuiltinPHPForProfile(profile Profile) []Entry {
	profile = normalizeProfile(profile)
	entries := make([]Entry, 0, len(stubPaths))
	for _, path := range stubPaths {
		if !includeStubPath(path, profile) {
			continue
		}
		entries = append(entries, Entry{
			Path:    path,
			Content: string(mustEmbedded(fs.ReadFile(phpFS, path))),
		})
	}
	return entries
}

func embeddedStubPaths(fsys fs.FS) []string {
	var paths []string
	err := fs.WalkDir(fsys, "php", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".php") {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	return mustEmbedded(paths, err)
}

func mustEmbedded[T any](v T, err error) T {
	if err != nil {
		panic(fmt.Sprintf("the embedded builtin stubs are unreadable, a build defect: %v", err))
	}
	return v
}

func normalizeProfile(profile Profile) Profile {
	if strings.TrimSpace(profile.PHPVersion) == "" {
		profile.PHPVersion = "8.6"
	}

	seen := make(map[string]struct{}, len(profile.Extensions))
	extensions := make([]string, 0, len(profile.Extensions))
	for _, ext := range profile.Extensions {
		ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), "ext-")
		if ext == "" {
			continue
		}
		if _, ok := seen[ext]; ok {
			continue
		}
		seen[ext] = struct{}{}
		extensions = append(extensions, ext)
	}
	sort.Strings(extensions)
	profile.Extensions = extensions
	return profile
}

func includeStubPath(stubPath string, profile Profile) bool {
	parts := strings.Split(stubPath, "/")
	if len(parts) < 3 || parts[0] != "php" {
		return false
	}

	switch parts[1] {
	case "core":
		return stubVersionAllowed(path.Base(stubPath), profile.PHPVersion)
	case "extensions":
		if len(parts) < 4 {
			return false
		}
		if !profileHasExtension(profile, parts[2]) {
			return false
		}
		return stubVersionAllowed(path.Base(stubPath), profile.PHPVersion)
	default:
		return false
	}
}

func profileHasExtension(profile Profile, ext string) bool {
	for _, enabled := range profile.Extensions {
		if enabled == ext {
			return true
		}
	}
	return false
}

func stubVersionAllowed(filename string, target string) bool {
	if !strings.HasPrefix(filename, "php-") || !strings.HasSuffix(filename, ".php") {
		return false
	}
	version := strings.TrimSuffix(strings.TrimPrefix(filename, "php-"), ".php")
	return compareMajorMinor(version, target) <= 0
}

func compareMajorMinor(left string, right string) int {
	leftMajor, leftMinor := parseMajorMinor(left)
	rightMajor, rightMinor := parseMajorMinor(right)
	if leftMajor != rightMajor {
		if leftMajor < rightMajor {
			return -1
		}
		return 1
	}
	if leftMinor != rightMinor {
		if leftMinor < rightMinor {
			return -1
		}
		return 1
	}
	return 0
}

func parseMajorMinor(version string) (int, int) {
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return 0, 0
	}
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return major, minor
}
