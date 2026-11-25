package document

import (
	"strings"
	"testing"
)

func TestNegotiate(t *testing.T) {
	cases := []struct {
		offered []string
		want    Encoding
	}{
		{nil, UTF16},
		{[]string{"utf-16"}, UTF16},
		{[]string{"utf-32", "utf-16"}, UTF16},
		{[]string{"utf-16", "utf-8"}, UTF8},
		{[]string{"utf-8"}, UTF8},
	}
	for _, c := range cases {
		if got := Negotiate(c.offered); got != c.want {
			t.Errorf("Negotiate(%q) = %v, want %v", c.offered, got, c.want)
		}
	}
}

func TestEncodingString(t *testing.T) {
	if UTF16.String() != "utf-16" || UTF8.String() != "utf-8" {
		t.Fatalf("got %q and %q", UTF16.String(), UTF8.String())
	}
}

func TestColumnsUTF16(t *testing.T) {
	text := "<?php\n$a = 'Größe'; $b;\n$e = '😀'; $f;\n$c = '漢字'; $d;\r\nplain $x;"
	cols := NewColumns(text, UTF16)
	cases := []struct {
		name         string
		line, bytes  int
		units        int
		roundTripped bool
	}{
		{"ascii line", 0, 3, 3, true},
		{"before the umlaut", 1, 8, 8, true},
		{"after Größe", 1, 16, 14, true},
		{"end of line 1", 1, 21, 19, true},
		{"after the emoji", 2, 10, 8, true},
		{"after the emoji variable", 2, 14, 12, true},
		{"after CJK", 3, 12, 8, true},
		{"before CR", 3, 17, 13, true},
		{"after the line end", 1, 25, 23, true},
		{"ascii line after non-ascii lines", 4, 7, 7, true},
		{"line past the end", 9, 4, 4, true},
		{"column zero", 2, 0, 0, true},
		{"inside a two-byte rune", 1, 9, 8, false},
		{"inside the emoji", 2, 8, 6, false},
	}
	for _, c := range cases {
		if got := cols.ToClient(c.line, c.bytes); got != c.units {
			t.Errorf("%s: ToClient(%d, %d) = %d, want %d", c.name, c.line, c.bytes, got, c.units)
		}
		if !c.roundTripped {
			continue
		}
		if got := cols.FromClient(c.line, c.units); got != c.bytes {
			t.Errorf("%s: FromClient(%d, %d) = %d, want %d", c.name, c.line, c.units, got, c.bytes)
		}
	}
	if got := cols.FromClient(2, 7); got != 6 {
		t.Errorf("FromClient inside a surrogate pair = %d, want the start of the emoji, 6", got)
	}
}

func TestColumnsUTF8IsIdentity(t *testing.T) {
	cols := NewColumns("$a = 'Größe'; $b;", UTF8)
	for col := 0; col < 20; col++ {
		if got := cols.ToClient(0, col); got != col {
			t.Errorf("ToClient(0, %d) = %d", col, got)
		}
		if got := cols.FromClient(0, col); got != col {
			t.Errorf("FromClient(0, %d) = %d", col, got)
		}
	}
}

func TestColumnsInvalidUTF8CountsOneUnitPerByte(t *testing.T) {
	cols := NewColumns("a\xffb é", UTF16)
	if got := cols.ToClient(0, 3); got != 3 {
		t.Errorf("ToClient = %d, want 3", got)
	}
	if got := cols.ToClient(0, 6); got != 5 {
		t.Errorf("ToClient after é = %d, want 5", got)
	}
}

func TestIsASCII(t *testing.T) {
	long := strings.Repeat("abcdefgh", 9)
	for i := range len(long) + 1 {
		mixed := long[:i] + "é" + long[i:]
		if isASCII(mixed) {
			t.Fatalf("isASCII(%q) = true", mixed)
		}
	}
	if !isASCII(long) || !isASCII("") {
		t.Fatal("isASCII rejects ASCII text")
	}
}

func BenchmarkColumnsToClient(b *testing.B) {
	line := "        $value = $this->repository->findOneBy(['name' => $name]);\n"
	for _, bc := range []struct {
		name string
		text string
	}{
		{"ascii", strings.Repeat(line, 10000)},
		{"non-ascii", strings.Repeat(line, 5000) + strings.Repeat("        $msg = 'Größe 😀'; "+line, 5000)},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				cols := NewColumns(bc.text, UTF16)
				for l := 0; l < 10000; l += 7 {
					cols.ToClient(l, 40)
				}
			}
		})
	}
}
