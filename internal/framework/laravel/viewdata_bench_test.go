package laravel

import (
	"strings"
	"testing"
)

func BenchmarkParseViewFile(b *testing.B) {
	src := controllerSrc + strings.Repeat(strings.TrimPrefix(controllerSrc, "<?php\n"), 20)
	b.ReportAllocs()
	for b.Loop() {
		parseViewFile("file:///app/app/Http/Controllers/UserController.php", src)
	}
}
