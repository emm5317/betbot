package adapters

import (
	"fmt"

	"betbot/internal/execution"
	"betbot/internal/execution/adapters/paper"
)

// NewBookAdapter returns the configured adapter implementation.
// Live adapters fail closed until their network/client paths are implemented.
func NewBookAdapter(name string) (execution.BookAdapter, error) {
	switch normalized := execution.NormalizeAdapterName(name); normalized {
	case "", execution.AdapterPaper:
		return paper.New(), nil
	case execution.AdapterDraftKings, execution.AdapterFanDuel, execution.AdapterBetMGM, execution.AdapterPinnacle:
		return nil, fmt.Errorf("execution adapter %q is configured but not implemented", normalized)
	default:
		return nil, fmt.Errorf("unsupported execution adapter %q", name)
	}
}
