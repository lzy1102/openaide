package kernel

import "log/slog"

// SagaStep represents a single operation in a saga with a compensation function.
type SagaStep struct {
	Name    string
	Execute func() error        // forward operation
	Compensate func() error     // undo (best-effort)
}

// RunSaga executes a series of steps sequentially. If any step fails,
// compensation functions run in reverse order for completed steps.
// Failed steps' compensations are skipped (they didn't complete).
// Returns the first error encountered.
func RunSaga(steps []SagaStep) error {
	completed := 0
	for i, step := range steps {
		if err := step.Execute(); err != nil {
			slog.Warn("Saga step failed, compensating", "step", step.Name, "error", err)
			// Compensate in reverse order
			for j := i - 1; j >= 0; j-- {
				if steps[j].Compensate != nil {
					if cerr := steps[j].Compensate(); cerr != nil {
						slog.Warn("Saga compensation failed", "step", steps[j].Name, "error", cerr)
					}
				}
			}
			return err
		}
		completed++
	}
	_ = completed
	return nil
}
