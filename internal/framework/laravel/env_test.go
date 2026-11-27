package laravel

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func writeEnvFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func envFilesOf(defs []EnvDefinition) []string {
	files := make([]string, len(defs))
	for i, def := range defs {
		files[i] = def.File
	}
	return files
}

func TestEnvKeysReadTheDotenvFilesLaravelLoads(t *testing.T) {
	root := t.TempDir()
	writeEnvFiles(t, root, map[string]string{
		".env":                      "APP_NAME=Shop\nDB_PASSWORD=hunter2\n",
		".env.example":              "APP_NAME=Laravel\nDB_PASSWORD=\nMAIL_HOST=smtp\n",
		".env.testing":              "export TEST_TOKEN=abc\n",
		".env.dusk.local":           "DUSK_DRIVER_URL=http://localhost:9515\n",
		".env.production.encrypted": "eyJpdiI6IkVOQ1JZUFRFRCJ9\nNOT_READ=1\n",
		".envrc":                    "NOT_READ_EITHER=1\n",
		"notes.env":                 "NOR_THIS=1\n",
	})
	keys := NewEnvKeys(root)
	if want := []string{"APP_NAME", "DB_PASSWORD", "DUSK_DRIVER_URL", "MAIL_HOST", "TEST_TOKEN"}; !slices.Equal(keys.Names(), want) {
		t.Errorf("Names() = %v, want %v", keys.Names(), want)
	}
	if files := envFilesOf(keys.Definitions("APP_NAME")); !slices.Equal(files, []string{".env", ".env.example"}) {
		t.Errorf("APP_NAME is set in %v", files)
	}
	loc := keys.Definition("MAIL_HOST")
	want := EnvDefinition{File: ".env.example", URI: symbols.PathToURI(filepath.Join(root, ".env.example")), Range: textpos.Range{
		Start: textpos.Position{Line: 2, Character: 0}, End: textpos.Position{Line: 2, Character: len("MAIL_HOST")},
	}}
	if loc == nil || *loc != want {
		t.Errorf("a key only in .env.example goes there: %+v, want %+v", loc, want)
	}
	if loc := keys.Definition("APP_NAME"); loc == nil || !strings.HasSuffix(loc.URI, "/.env") {
		t.Errorf("a key in .env goes to .env first: %+v", loc)
	}
	if loc := keys.Definition("TEST_TOKEN"); loc == nil || loc.Range.Start.Character != len("export ") {
		t.Errorf("an exported key is found after export: %+v", loc)
	}
}

func TestEnvKeysAKeySetTwiceGoesToTheAssignmentThatWins(t *testing.T) {
	root := t.TempDir()
	writeEnvFiles(t, root, map[string]string{".env": "APP_ENV=local\nAPP_ENV=production\n"})
	keys := NewEnvKeys(root)
	defs := keys.Definitions("APP_ENV")
	if len(defs) != 1 || defs[0].Range.Start.Line != 1 {
		t.Errorf("one definition, on the last line, as phpdotenv keeps the last value: %+v", defs)
	}
}

func TestEnvKeysAMultiLineValueAddsNoKeys(t *testing.T) {
	root := t.TempDir()
	writeEnvFiles(t, root, map[string]string{".env": "JWT_KEY=\"-----BEGIN-----\nFAKE_KEY=inside\n-----END-----\"\nAFTER=1\n"})
	if names := NewEnvKeys(root).Names(); !slices.Equal(names, []string{"AFTER", "JWT_KEY"}) {
		t.Errorf("Names() = %v", names)
	}
}

func TestEnvKeysFollowChangesToTheDotenvFiles(t *testing.T) {
	root := t.TempDir()
	writeEnvFiles(t, root, map[string]string{".env": "APP_NAME=Shop\n"})
	keys := NewEnvKeys(root)
	if names := keys.Names(); !slices.Equal(names, []string{"APP_NAME"}) {
		t.Fatalf("Names() = %v", names)
	}

	writeEnvFiles(t, root, map[string]string{".env": "APP_NAME=Shop\nAPP_DEBUG=true\n", ".env.testing": "DB_DATABASE=testing\n"})
	keys.FilesChanged([]string{filepath.Join(root, "app", "Models", "User.php"), filepath.Join(root, "sub", ".env")})
	if names := keys.Names(); !slices.Equal(names, []string{"APP_NAME"}) {
		t.Errorf("unrelated paths and a .env outside the root keep the keys read: %v", names)
	}
	keys.FilesChanged([]string{filepath.Join(root, ".env")})
	if names := keys.Names(); !slices.Equal(names, []string{"APP_DEBUG", "APP_NAME", "DB_DATABASE"}) {
		t.Errorf("a changed .env is read again: %v", names)
	}

	if err := os.Remove(filepath.Join(root, ".env.testing")); err != nil {
		t.Fatal(err)
	}
	keys.FilesChanged([]string{filepath.Join(root, ".env.testing")})
	if names := keys.Names(); slices.Contains(names, "DB_DATABASE") {
		t.Errorf("a deleted file's keys are forgotten: %v", names)
	}
}

func TestEnvKeysMatchingIgnoresCaseAndNamesTheFiles(t *testing.T) {
	root := t.TempDir()
	writeEnvFiles(t, root, map[string]string{".env": "APP_NAME=Shop\nDB_HOST=db\n", ".env.example": "APP_NAME=Laravel\nAPP_URL=x\n"})
	got := NewEnvKeys(root).Matching("app_")
	want := []EnvKey{{Name: "APP_NAME", DefinedIn: ".env, .env.example"}, {Name: "APP_URL", DefinedIn: ".env.example"}}
	if !slices.Equal(got, want) {
		t.Errorf("Matching(app_) = %+v, want %+v", got, want)
	}
}

func TestReadsEnvNamesLaravelsEnvReaders(t *testing.T) {
	cases := []struct {
		function, class, method string
		want                    bool
	}{
		{"env", "", "", true},
		{"ENV", "", "", true},
		{"App\\Support\\env", "", "", false},
		{"config", "", "", false},
		{"", "Illuminate\\Support\\Env", "get", true},
		{"", "Illuminate\\Support\\Env", "getOrFail", true},
		{"", "Illuminate\\Support\\Env", "set", false},
		{"", "Env", "get", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		if got := ReadsEnv(tc.function, tc.class, tc.method); got != tc.want {
			t.Errorf("ReadsEnv(%q, %q, %q) = %v, want %v", tc.function, tc.class, tc.method, got, tc.want)
		}
	}
}

func TestEnvKeysHoverNamesTheFilesNotTheValues(t *testing.T) {
	root := t.TempDir()
	writeEnvFiles(t, root, map[string]string{".env": "DB_PASSWORD=hunter2\n", ".env.example": "DB_PASSWORD=example-secret\n"})
	keys := NewEnvKeys(root)
	hover := keys.Hover("DB_PASSWORD")
	if !strings.Contains(hover, "`.env`, `.env.example`") {
		t.Errorf("hover names the files: %q", hover)
	}
	if strings.Contains(hover, "hunter2") || strings.Contains(hover, "example-secret") {
		t.Errorf("hover never shows a value: %q", hover)
	}
	if hover := keys.Hover("DB_PASWORD"); !strings.Contains(hover, "Not set") {
		t.Errorf("hover says a mistyped key is set nowhere: %q", hover)
	}
}
