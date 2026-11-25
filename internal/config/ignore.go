package config

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/rule"
)

type IgnoreRules struct {
	Rules   []IgnoreRule
	Invalid []error
}

func (r *IgnoreRules) UnmarshalJSON(data []byte) error {
	var entries []json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		*r = IgnoreRules{Invalid: []error{fmt.Errorf("diagnosticIgnore must be a list of rules, got %s: %w", data, err)}}
		return nil
	}
	parsed := IgnoreRules{}
	for _, entry := range entries {
		var rule IgnoreRule
		if err := json.Unmarshal(entry, &rule); err != nil {
			parsed.Invalid = append(parsed.Invalid, err)
			continue
		}
		parsed.Rules = append(parsed.Rules, rule)
	}
	*r = parsed
	return nil
}

func (r IgnoreRules) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Rules)
}

type IgnoreRule struct {
	target  *rule.Target
	pattern string
	glob    []string
	message *regexp.Regexp
}

type ignoreRuleJSON struct {
	Code    string `json:"code,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message,omitempty"`
}

func (r *IgnoreRule) UnmarshalJSON(data []byte) error {
	var raw ignoreRuleJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("diagnosticIgnore entry %s: %w", data, err)
	}
	if raw.Code == "" && raw.Path == "" && raw.Message == "" {
		return fmt.Errorf("diagnosticIgnore entry %s: needs at least one of code, path or message", data)
	}
	parsed := IgnoreRule{pattern: raw.Path}
	if raw.Code != "" {
		target, err := rule.ParseTarget(raw.Code)
		if err != nil {
			return fmt.Errorf("diagnosticIgnore code: %w", err)
		}
		parsed.target = &target
	}
	if raw.Path != "" {
		glob, err := parseGlob(raw.Path)
		if err != nil {
			return err
		}
		parsed.glob = glob
	}
	if raw.Message != "" {
		re, err := regexp.Compile(raw.Message)
		if err != nil {
			return fmt.Errorf("diagnosticIgnore message %q: %w", raw.Message, err)
		}
		parsed.message = re
	}
	*r = parsed
	return nil
}

func (r IgnoreRule) MarshalJSON() ([]byte, error) {
	raw := ignoreRuleJSON{Path: r.pattern}
	if r.target != nil {
		raw.Code = r.target.String()
	}
	if r.message != nil {
		raw.Message = r.message.String()
	}
	return json.Marshal(raw)
}

func (r IgnoreRule) HasPath() bool {
	return r.glob != nil
}

func (r IgnoreRule) MatchesPath(relPath string) bool {
	if r.glob == nil {
		return true
	}
	segments := strings.Split(relPath, "/")
	for n := len(segments); n > 0; n-- {
		if matchGlob(r.glob, segments[:n]) {
			return true
		}
	}
	return false
}

func parseGlob(pattern string) ([]string, error) {
	normalized := strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(pattern), `\`, "/"), "./")
	anchored := strings.HasPrefix(normalized, "/")
	normalized = strings.Trim(normalized, "/")
	if normalized == "" {
		return nil, fmt.Errorf("diagnosticIgnore path %q: names no file or directory", pattern)
	}
	if !anchored && !strings.Contains(normalized, "/") {
		normalized = "**/" + normalized
	}
	glob := strings.Split(normalized, "/")
	for _, segment := range glob {
		if _, err := path.Match(segment, ""); err != nil {
			return nil, fmt.Errorf("diagnosticIgnore path %q: %w", pattern, err)
		}
	}
	return glob, nil
}

func (r IgnoreRule) Matches(source, code, message string) bool {
	return (r.target == nil || r.target.Matches(source, code)) && (r.message == nil || r.message.MatchString(message))
}

func matchGlob(pattern, segments []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			for skip := 0; skip <= len(segments); skip++ {
				if matchGlob(pattern[1:], segments[skip:]) {
					return true
				}
			}
			return false
		}
		if len(segments) == 0 {
			return false
		}
		if ok, _ := path.Match(pattern[0], segments[0]); !ok {
			return false
		}
		pattern, segments = pattern[1:], segments[1:]
	}
	return len(segments) == 0
}
