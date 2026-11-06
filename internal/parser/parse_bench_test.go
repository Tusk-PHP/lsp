package parser

import (
	"strings"
	"testing"
)

// benchSources are a plain class file and a template that mixes PHP with
// HTML and JavaScript, each repeated to a realistic large-file size.
var benchSources = map[string]string{
	"class": "<?php\nnamespace App;\n\n" + strings.Repeat(`/** Service doc. */
final class Service%s extends Base implements Contract {
    private array $items = [];
    public function __construct(private readonly Logger $logger) {}
    public function run(int $limit = 10): array {
        foreach ($this->items as $key => $item) {
            if ($item->ready() && $key < $limit) { $this->logger->info("ready {$key}"); }
        }
        return array_map(fn ($x) => $x * 2, $this->items);
    }
}
`, 200),
	"template": strings.Repeat(`<div class="wrap">
<?php foreach ( $posts as $post ) : ?>
	<h2><?php echo esc_html( $post->post_title ); ?></h2>
	<script>
	jQuery( function( $ ) { $( '#id' ).on( 'click', function() { foo( 1, 2 ); } ); } );
	</script>
<?php endforeach; ?>
</div>
`, 400),
}

func BenchmarkParse(b *testing.B) {
	for name, src := range benchSources {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				New().Parse(src)
			}
		})
	}
}
