package lexer

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/phpref"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

const tokensScript = `    $out = '';
    foreach (token_get_all($source) as $t) {
        [$name, $text] = is_array($t) ? [token_name($t[0]), $t[1]] : [$t, $t];
        $out .= $name . "\0" . $text . "\1";
    }
    echo $path, "\t", ($argv[1] ?? '') === 'detail' ? base64_encode($out) : sha1($out), "\n";`

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

func TestLexerMatchesPHPTokenizer(t *testing.T) {
	refs := phpref.Versions(t)
	sources := phpref.ReadAll(t, phpref.Corpus(t))
	for _, php := range refs {
		t.Run(php.Version.String(), func(t *testing.T) {
			want := php.Run(t, tokensScript, sources)
			mismatched := map[string]string{}
			for path, src := range sources {
				sum := sha1.Sum([]byte(serialize(src, php.Version)))
				if hex.EncodeToString(sum[:]) != want[path] && len(mismatched) < 5 {
					mismatched[path] = src
				}
			}
			if len(mismatched) == 0 {
				return
			}
			t.Errorf("files lex differently from PHP %s", php.Version)
			for path, encoded := range php.Run(t, tokensScript, mismatched, "detail") {
				t.Errorf("%s: %s", path, firstDifference(serialize(mismatched[path], php.Version), decode(t, encoded)))
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
