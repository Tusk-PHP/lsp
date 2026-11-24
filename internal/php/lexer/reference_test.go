package lexer

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const referenceScript = `<?php
$detail = ($argv[1] ?? '') === 'detail';
while (($path = fgets(STDIN)) !== false) {
    $path = rtrim($path, "\n");
    $length = (int) fgets(STDIN);
    $source = $length > 0 ? stream_get_contents(STDIN, $length) : '';
    $out = '';
    foreach (token_get_all($source) as $t) {
        [$name, $text] = is_array($t) ? [token_name($t[0]), $t[1]] : [$t, $t];
        $out .= $name . "\0" . $text . "\1";
    }
    echo $path, "\t", $detail ? base64_encode($out) : sha1($out), "\n";
}
`

type reference struct {
	command []string
	version token.Version
}

func references(t *testing.T) []reference {
	t.Helper()
	commands := os.Getenv("TUSK_PHP_TOKENIZERS")
	if commands == "" {
		commands = "php"
	}
	var refs []reference
	for _, command := range strings.Split(commands, ";") {
		argv := strings.Fields(command)
		out, err := exec.Command(argv[0], append(argv[1:], "-r", `if (!function_exists("token_get_all")) exit(1); echo PHP_MAJOR_VERSION, ".", PHP_MINOR_VERSION;`)...).Output()
		if err != nil {
			continue
		}
		version, err := token.ParseVersion(string(out))
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		if version.String() != string(out) {
			t.Logf("skipping %s: PHP %s is outside the lexer's 8.0 to %s", command, out, token.Latest)
			continue
		}
		refs = append(refs, reference{argv, version})
	}
	if len(refs) == 0 {
		t.Skip("no PHP with the tokenizer extension (set TUSK_PHP_TOKENIZERS to commands separated by ';')")
	}
	return refs
}

func serialize(src string, version token.Version) string {
	var b strings.Builder
	for _, tok := range Lex(src, version) {
		if tok.Kind.IsCharacter() {
			b.WriteString(tok.Text(src))
		} else {
			b.WriteString(tok.Kind.String())
		}
		b.WriteByte(0)
		b.WriteString(tok.Text(src))
		b.WriteByte(1)
	}
	return b.String()
}

func corpus(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, root := range []string{"../../../testdata/project", "../../../testdata/laravel/vendor", "../../../testdata/symfony/vendor"} {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".php") {
				abs, _ := filepath.Abs(path)
				files = append(files, abs)
			}
			return nil
		})
	}
	if len(files) < 100 {
		t.Skip("the Laravel and Symfony fixtures' vendor trees aren't installed")
	}
	return files
}

func runReference(t *testing.T, php reference, files []string, detail bool) map[string]string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "tokens.php")
	if err := os.WriteFile(script, []byte(referenceScript), 0o644); err != nil {
		t.Fatal(err)
	}
	args := append(append([]string{}, php.command[1:]...), "-d", "short_open_tag=0", "-d", "memory_limit=-1", script)
	if detail {
		args = append(args, "detail")
	}
	cmd := exec.Command(php.command[0], args...)
	var input strings.Builder
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		input.WriteString(path + "\n" + strconv.Itoa(len(data)) + "\n")
		input.Write(data)
	}
	cmd.Stdin = strings.NewReader(input.String())
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running PHP's tokenizer: %v", err)
	}
	results := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(nil, 1<<30)
	for scanner.Scan() {
		path, sum, _ := strings.Cut(scanner.Text(), "\t")
		results[path] = sum
	}
	return results
}

func TestLexerMatchesPHPTokenizer(t *testing.T) {
	refs := references(t)
	files := corpus(t)
	for _, php := range refs {
		t.Run(php.version.String(), func(t *testing.T) {
			want := runReference(t, php, files, false)
			var mismatched []string
			for _, path := range files {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				sum := sha1.Sum([]byte(serialize(string(data), php.version)))
				if hex.EncodeToString(sum[:]) != want[path] {
					mismatched = append(mismatched, path)
				}
			}
			if len(mismatched) == 0 {
				return
			}
			t.Errorf("%d of %d files lex differently from PHP %s", len(mismatched), len(files), php.version)
			details := runReference(t, php, mismatched[:min(5, len(mismatched))], true)
			for path, encoded := range details {
				data, _ := os.ReadFile(path)
				t.Errorf("%s: %s", path, firstDifference(serialize(string(data), php.version), decode(t, encoded)))
			}
		})
	}
}

func decode(t *testing.T, encoded string) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decoding PHP's tokens: %v", err)
	}
	return string(data)
}

func firstDifference(got, want string) string {
	gotTokens := strings.Split(got, "\x01")
	wantTokens := strings.Split(want, "\x01")
	for i := range min(len(gotTokens), len(wantTokens)) {
		if gotTokens[i] != wantTokens[i] {
			return "token " + strconv.Itoa(i) + ": got " + quote(gotTokens[i]) + ", PHP has " + quote(wantTokens[i])
		}
	}
	return "token counts differ: " + strconv.Itoa(len(gotTokens)) + " vs " + strconv.Itoa(len(wantTokens))
}

func quote(tok string) string {
	name, text, _ := strings.Cut(tok, "\x00")
	if len(text) > 60 {
		text = text[:60] + "…"
	}
	return name + " " + strconv.Quote(text)
}
