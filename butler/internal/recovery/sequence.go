package recovery

import (
	"context"
	"fmt"
)

type Sequence struct {
	Steps []Bootstrapper
}

func (s Sequence) Reconcile(ctx context.Context) error {
	for index, step := range s.Steps {
		if step == nil {
			continue
		}
		if err := step.Reconcile(ctx); err != nil {
			return fmt.Errorf("identity bootstrap step %d: %w", index+1, err)
		}
	}
	return nil
}
