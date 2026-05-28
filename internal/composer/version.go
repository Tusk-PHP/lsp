package composer

import (
	"strconv"
	"strings"
)

// MinPHPFromConstraint returns the minimum PHP "MAJOR.MINOR" version implied
// by a composer-style version constraint such as ">=8.1", "^7.4", or
// "^8.2 || ^7.4". Returns "" when the constraint has no parseable version.
//
// This is the public name for the same logic that drives
// GetPlatform's PHP-version detection.
func MinPHPFromConstraint(constraint string) string {
	return minimumPHPVersion(constraint)
}

// CompareConstraint reports whether projectVersion ("MAJOR.MINOR") satisfies
// the minimum implied by packageConstraint. Both inputs are treated as
// "MAJOR.MINOR" tuples; patch components and upper bounds are ignored — the
// check is "is the project's PHP at least the package's floor?".
//
// Returns:
//
//	+1 when projectVersion >= packageConstraint floor
//	 0 when either side cannot be parsed (unknown — caller should treat as
//	   "undetermined" and omit the compatibility glyph)
//	-1 when projectVersion < packageConstraint floor
func CompareConstraint(projectVersion, packageConstraint string) int {
	pkgMin := minimumPHPVersion(packageConstraint)
	if pkgMin == "" || projectVersion == "" {
		return 0
	}
	projMajor, projMinor, ok := parseMajorMinor(projectVersion)
	if !ok {
		return 0
	}
	pkgMajor, pkgMinor, ok := parseMajorMinor(pkgMin)
	if !ok {
		return 0
	}
	switch {
	case projMajor > pkgMajor:
		return +1
	case projMajor < pkgMajor:
		return -1
	case projMinor > pkgMinor:
		return +1
	case projMinor < pkgMinor:
		return -1
	}
	return +1
}

// normalizeMajorMinor trims a "MAJOR.MINOR.PATCH" or "vMAJOR.MINOR" string to
// "MAJOR.MINOR" form. Returns "" on parse failure.
func normalizeMajorMinor(version string) string {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	major, minor, ok := parseMajorMinor(v)
	if !ok {
		return ""
	}
	return strconv.Itoa(major) + "." + strconv.Itoa(minor)
}
