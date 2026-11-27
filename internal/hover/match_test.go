package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverVariableAssignedFromMatch(t *testing.T) {
	source := `<?php
namespace Sylius\Bundle\AdminBundle\Twig\Component\Dashboard;

class StatisticsComponent {
    public string $interval = 'day';

    private function resolveInterval(): \DateInterval
    {
        $interval = match ($this->interval) {
            'day' => 'P1D',
            'month' => 'P1M',
            default => throw new \InvalidArgumentException(sprintf('Interval "%s" is not supported.', $this->interval)),
        };

        return new \DateInterval($interval);
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/CategoryController.php", source)
	p := NewProvider(idx, nil, "")
	if got := hoverAt(t, p, source, "return new \\DateInterval($interval);", "$interval"); !strings.Contains(got, "string $interval") {
		t.Errorf("hover on $interval: want %q in\n%s", "string $interval", got)
	}
}
