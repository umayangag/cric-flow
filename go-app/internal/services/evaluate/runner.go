package evaluate

import (
	"context"
	"fmt"
	"io"
	"os"
)

// Runner orchestrates the evaluate command by loading inputs and computing metrics.
type Runner struct {
	Repo EvaluationRepo
	Out  io.Writer
}

// Run executes the evaluation and writes a single-line summary to Out.
func (r Runner) Run(ctx context.Context, opts Options) error {
	out := r.Out
	if out == nil {
		out = os.Stdout
	}

	inputs, err := r.Repo.LoadInputs(ctx, opts.Season, opts.Format)
	if err != nil {
		return err
	}

	m := ComputeMetrics(inputs)
	_, _ = fmt.Fprintf(out, "MAE=%.4f RMSE=%.4f Brier=%.4f\n", m.MAE, m.RMSE, m.Brier)
	return nil
}
