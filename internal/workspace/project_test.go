package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectReadsTheConfiguredFramework(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(`{"framework":"symfony","phpVersion":"8.2"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var logged []string
	project, err := LoadProject(root, func(msg string) { logged = append(logged, msg) })
	if err != nil {
		t.Fatal(err)
	}
	if project.Framework != "symfony" {
		t.Errorf("Framework = %q, want symfony", project.Framework)
	}
	if project.Config == nil || project.Profile.PHPVersion == "" || project.ProfileSource == "" {
		t.Errorf("incomplete project: %+v", project)
	}
	if len(logged) == 0 {
		t.Error("profile detection reported nothing")
	}
}

func TestLoadProjectDetectsTheFramework(t *testing.T) {
	project, err := LoadProject(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if project.Framework == "" {
		t.Error("no framework detected")
	}
}

func TestLoadProjectFailsOnABrokenConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProject(root, nil); err == nil {
		t.Error("loaded a configuration that doesn't parse")
	}
}
