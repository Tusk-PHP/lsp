package parser

import "testing"

func BenchmarkParseDocBlock(b *testing.B) {
	docs := map[string]string{
		"single-line": `/**
 * Initialize the product.
 *
 * @param int $id
 * @param array<string, mixed> $data The data to fill it with
 * @return bool
 * @throws \RuntimeException
 */`,
		"multi-line": `/**
 * Load a row.
 *
 * @param array{
 *     id: int,
 *     tags: list<string>,
 * } $row
 * @return array{
 *     id: int,
 *     name: string,
 * }
 */`,
	}
	for _, name := range []string{"single-line", "multi-line"} {
		doc := docs[name]
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ParseDocBlock(doc)
			}
		})
	}
}
