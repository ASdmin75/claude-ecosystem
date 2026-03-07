package pipeline

import (
	"context"
	"fmt"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

// Run decides which mode to use based on p.EffectiveMode() and delegates
// to the appropriate implementation.
func (r *Runner) Run(ctx context.Context, p config.Pipeline) (string, error) {
	switch p.EffectiveMode() {
	case "sequential":
		return r.RunSequential(ctx, p)
	case "parallel":
		return r.RunParallel(ctx, p)
	default:
		return "", fmt.Errorf("pipeline %s: unknown mode %q", p.Name, p.EffectiveMode())
	}
}
