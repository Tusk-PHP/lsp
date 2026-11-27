package symbols

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func modelFile(version, classes, methods int) string {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App\\Models;\n\nuse Illuminate\\Database\\Eloquent\\Model;\nuse Illuminate\\Support\\Collection;\n\n")
	for c := range classes {
		src.WriteString("/**\n * @template T of Model\n * @property int $id\n * @property string $name\n * @property-read Collection<int, Model> $items\n * @method static Collection<int, static> active()\n */\n")
		fmt.Fprintf(&src, "class Model%d extends Model implements \\JsonSerializable\n{\n    protected array $casts = [];\n    public const STATUS_%d = 'v%d';\n", c, c, version)
		for m := range methods {
			fmt.Fprintf(&src, "    /**\n     * @param Collection<int, Model> $items\n     * @return Collection<int, Model>\n     */\n    public function method%d_%d(Collection $items, ?string $name = null, int ...$ids): Collection\n    {\n        return $items;\n    }\n", m, version)
		}
		src.WriteString("}\n\n")
	}
	return src.String()
}

func otherModels(classes int) string {
	return strings.ReplaceAll(modelFile(2, classes, 5), "class Model", "class Other")
}

func BenchmarkReindexClassFile(b *testing.B) {
	texts := [2]string{modelFile(0, 20, 20), modelFile(1, 20, 20)}
	idx := NewIndex()
	idx.IndexFile("file:///other.php", otherModels(200))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		idx.IndexFile("file:///reindexed.php", texts[i%2])
		i++
	}
}

func BenchmarkLookupWhileReindexing(b *testing.B) {
	texts := [2]string{modelFile(0, 20, 20), modelFile(1, 20, 20)}
	idx := NewIndex()
	idx.IndexFile("file:///other.php", otherModels(50))
	idx.IndexFile("file:///reindexed.php", texts[0])
	done := make(chan struct{})
	reindexes := 0
	var wg sync.WaitGroup
	wg.Go(func() {
		for ; ; reindexes++ {
			select {
			case <-done:
				return
			default:
				idx.IndexFile("file:///reindexed.php", texts[reindexes%2])
			}
		}
	})
	waits := make([]time.Duration, 0, 1<<20)
	for b.Loop() {
		start := time.Now()
		if idx.Lookup(`App\Models\Other7`) == nil {
			b.Fatal("lookup lost a class another file declares")
		}
		if len(waits) < cap(waits) {
			waits = append(waits, time.Since(start))
		}
	}
	close(done)
	wg.Wait()
	b.ReportMetric(float64(reindexes)/b.Elapsed().Seconds(), "reindex/s")
	slices.Sort(waits)
	b.ReportMetric(float64(waits[len(waits)*99/100].Nanoseconds()), "p99-ns")
	b.ReportMetric(float64(waits[len(waits)-1].Nanoseconds()), "max-ns")
}

func TestReindexSwapsAFileWholeForConcurrentReaders(t *testing.T) {
	const classes, methods = 5, 8
	texts := [2]string{modelFile(0, classes, methods), modelFile(1, classes, methods)}
	idx := NewIndex()
	idx.IndexFile("file:///models.php", texts[0])
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		defer close(done)
		for i := 1; i <= 200; i++ {
			idx.IndexFile("file:///models.php", texts[i%2])
		}
	})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				for c := range classes {
					checkModelVersion(t, idx, c, methods)
				}
			}
		})
	}
	wg.Wait()
}

func checkModelVersion(t *testing.T, idx *Index, class, methods int) {
	t.Helper()
	fqn := fmt.Sprintf(`App\Models\Model%d`, class)
	if idx.Lookup(fqn) == nil {
		t.Errorf("%s is missing while its file is re-indexed", fqn)
		return
	}
	versions := map[string]int{}
	virtual := 0
	for _, m := range idx.GetClassMembers(fqn) {
		if m.ParentFQN != fqn {
			continue
		}
		if m.IsVirtual {
			virtual++
		}
		if m.Kind == KindMethod && strings.HasPrefix(m.Name, "method") {
			versions[m.Name[strings.LastIndexByte(m.Name, '_')+1:]]++
		}
	}
	if len(versions) != 1 {
		t.Errorf("%s has methods from versions %v; want all from one version", fqn, versions)
		return
	}
	for v, n := range versions {
		if n != methods {
			t.Errorf("%s has %d methods of version %s; want %d", fqn, n, v, methods)
		}
	}
	if virtual != 4 {
		t.Errorf("%s has %d virtual members; want the 4 its docblock declares", fqn, virtual)
	}
}

func TestVirtualMemberNamedLikeARealOneIsLeftOut(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///user.php", `<?php
namespace App;
/**
 * @property string $email
 * @property int $age
 * @method void save()
 * @method void touch()
 * @method void touch()
 */
class User {
    public string $email;
    public function save(): bool { return true; }
}
`)
	user := idx.Lookup(`App\User`)
	if user == nil {
		t.Fatal(`App\User is not indexed`)
	}
	var got []string
	for _, c := range user.Children {
		got = append(got, fmt.Sprintf("%s virtual=%t", c.Name, c.IsVirtual))
	}
	want := []string{"$email virtual=false", "save virtual=false", "$age virtual=true", "touch virtual=true"}
	if !slices.Equal(got, want) {
		t.Errorf("children = %q; want %q", got, want)
	}
	if save := idx.Lookup(`App\User::save`); save == nil || save.IsVirtual || save.ReturnType != "bool" {
		t.Errorf(`App\User::save = %+v; want the declared method`, save)
	}
}

func BenchmarkReindexWriteLockHeld(b *testing.B) {
	const uri = "file:///reindexed.php"
	texts := [2]string{modelFile(0, 20, 20), modelFile(1, 20, 20)}
	files := [2]*parser.FileNode{parser.ParseFile(texts[0]), parser.ParseFile(texts[1])}
	idx := NewIndex()
	idx.IndexFile("file:///other.php", otherModels(200))
	idx.IndexFile(uri, texts[0])
	b.ReportAllocs()
	i := 1
	for b.Loop() {
		b.StopTimer()
		decls := fileDeclarations(uri, files[i%2])
		b.StartTimer()
		idx.mu.Lock()
		idx.replaceFileLocked(uri, indexedText{text: texts[i%2], from: SourceProject}, decls)
		idx.mu.Unlock()
		i++
	}
}
