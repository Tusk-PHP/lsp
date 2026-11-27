package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverDestructuredVariables(t *testing.T) {
	source := `<?php
namespace MageOS\AiBase\Model\Usage;

class UsageMaintenance {
    private function rollUpAndDeleteRawRows(\DateTimeZone $localTimezone, array $days): void
    {
        foreach ($days as $localDay) {
            [$windowStart, $windowEnd] = $this->utcWindowForLocalDay($localDay, $localTimezone);
            $this->aggregateRange($windowStart, $windowEnd);
        }
    }

    /**
     * @return array{0: \DateTimeImmutable, 1: \DateTimeImmutable}
     */
    private function utcWindowForLocalDay(
        \DateTimeImmutable $localDay,
        \DateTimeZone $localTimezone
    ): array {
        return [$localDay, $localDay];
    }

    private function resolveDates(string $period): void
    {
        [$startDate, $endDate] = match ($period) {
            'year' => [
                $this->createDateTime('first day of next month last year')->format('Y-m-d'),
                $this->createDateTime('first day of next month this year')->format('Y-m-d'),
            ],
            'month' => [
                $this->createDateTime('+1 day -1 month')->format('Y-m-d'),
                $this->createDateTime('+1 day')->format('Y-m-d'),
            ],
            default => throw new \InvalidArgumentException(sprintf('Period "%s" is not supported.', $period)),
        };

        $this->startDate = $startDate;
    }

    private function createDateTime(string $datetime): \DateTime
    {
        return new \DateTime($datetime);
    }
}
`
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///app/CategoryController.php", source)
	p := NewProvider(idx, nil, "")
	for _, c := range []struct{ line, word, want string }{
		{"[$windowStart, $windowEnd] = $this->utcWindowForLocalDay", "$windowStart", "DateTimeImmutable $windowStart"},
		{"$this->aggregateRange($windowStart, $windowEnd);", "$windowEnd", "DateTimeImmutable $windowEnd"},
		{"$this->startDate = $startDate;", "$startDate;", "string $startDate"},
	} {
		if got := hoverAt(t, p, source, c.line, c.word); !strings.Contains(got, c.want) {
			t.Errorf("hover on %s in %q: want %q in\n%s", c.word, c.line, c.want, got)
		}
	}
}
