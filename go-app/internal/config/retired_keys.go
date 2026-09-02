package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// retiredKey names a setting this repo used to honour, and says what happened to it.
//
// A retired key is refused, not ignored, for the same reason a retired request parameter
// is (see internal/services/apiparams): every one of these changed which model answered a
// prediction. A box whose config.json still says selection.win_model is asking for a
// choice that no longer exists, and starting anyway would serve a different answer than
// the operator configured, silently and for as long as nobody looked.
type retiredKey struct {
	// Path is the dotted JSON path, as it appears in config.json.
	Path string
	// RemovedIn names the migration step that removed it, so the message points at the
	// plan row a reader can go and read.
	RemovedIn string
	// Reason says what replaced it, or why nothing did.
	Reason string
}

// retiredKeys is the whole list. Adding an entry retires a setting for every deployment.
var retiredKeys = []retiredKey{
	{
		Path:      "selection.win_model",
		RemovedIn: "P-5",
		Reason: "the XI-responsive model is the only selection path; there is no windowed-form " +
			"model to switch back to",
	},
	{
		Path:      "selection.use_win_probability_selection",
		RemovedIn: "P-5",
		Reason:    "limited-overs selection always maximises win probability; TEST is always rating-ordered",
	},
	{
		Path:      "selection.use_optimizer",
		RemovedIn: "P-5",
		Reason:    "the per-call hill-climb over score weights is gone; the search runs in ml-service",
	},
	{
		Path:      "selection.score_weights",
		RemovedIn: "P-5",
		Reason:    "players are no longer scored by weighted batting / bowling / fielding points",
	},
	{
		Path:      "selection.score_weights_by_format",
		RemovedIn: "P-5",
		Reason:    "see selection.score_weights",
	},
	{
		Path:      "selection.score_normalization",
		RemovedIn: "P-5",
		Reason:    "see selection.score_weights",
	},
	{
		Path:      "selection.meta_model_path",
		RemovedIn: "P-5",
		Reason:    "the combination meta-model is gone; the objective is the XI win model",
	},
	{
		Path:      "selection.max_pool_size_for_full_enum",
		RemovedIn: "P-5",
		Reason:    "nothing enumerates XIs in go-app any more",
	},
	{
		Path:      "selection.max_win_prob_swap_iterations",
		RemovedIn: "P-5",
		Reason:    "the swap loop runs in ml-service, bounded by selection.max_win_prob_eval_budget",
	},
	{
		Path:      "selection.default_pool_csv",
		RemovedIn: "P-5",
		Reason:    "the pool comes from the database for the fixture's two sides",
	},
	{
		Path:      "selection.require_keeper",
		RemovedIn: "P-5",
		Reason: "it was never read; the prediction request's require_keeper field is what " +
			"constrains a selection",
	},
	{
		Path:      "predictor.default_extras",
		RemovedIn: "P-5",
		Reason: "the extras model went with the rescale; extras come from the simulator's as-of " +
			"extras-per-delivery rate",
	},
	{
		Path:      "predictor.simulation",
		RemovedIn: "P-5",
		Reason: "the Normal(mean, mean x CV) Monte Carlo is gone; the simulator draws from the " +
			"performance model's own distributions",
	},
	{
		Path:      "predictor.max_total_samples",
		RemovedIn: "P-5",
		Reason:    "see predictor.simulation",
	},
	{
		Path:      "predictor.simulation_top_k_per_team",
		RemovedIn: "P-5",
		Reason:    "see predictor.simulation",
	},
	{
		Path:      "predictor.simulation_num_samples_per_matchup",
		RemovedIn: "P-5",
		Reason:    "see predictor.simulation",
	},
	{
		Path:      "features",
		RemovedIn: "P-6",
		Reason: "the precompute pass and the export CSVs are gone; every feature the models " +
			"read is computed as an as-of accumulator inside the rating pass",
	},
	{
		Path:      "export",
		RemovedIn: "P-6",
		Reason:    "there is no export step; the rating pass writes the training frames into the run directory",
	},
	{
		Path:      "backtest",
		RemovedIn: "P-6",
		Reason: "the per-match evaluate flow and the contributions export went with the models " +
			"they scored (P-5); the backtest surface is L4's report",
	},
	{
		Path:      "weather",
		RemovedIn: "P-6",
		Reason:    "nothing has ever populated weather_data; the tables and their probes are dropped",
	},
	{
		Path:      "outputs.export_dir",
		RemovedIn: "P-6",
		Reason:    "renamed to outputs.dir: go-app writes no exports, only its resource observations",
	},
	{
		Path:      "pipeline.precompute_concurrency",
		RemovedIn: "P-6",
		Reason:    "import is the only pipeline go-app runs workers for; see pipeline.import_concurrency",
	},
	{
		Path:      "pipeline.seqcalc_concurrency",
		RemovedIn: "P-6",
		Reason:    "see pipeline.precompute_concurrency",
	},
	{
		Path:      "pipeline.export_concurrency",
		RemovedIn: "P-6",
		Reason:    "see pipeline.precompute_concurrency",
	},
	{
		Path:      "pipeline.fielding_concurrency",
		RemovedIn: "P-6",
		Reason:    "see pipeline.precompute_concurrency",
	},
	{
		Path:      "pipeline.precompute_eta_seconds_per_fmt",
		RemovedIn: "P-6",
		Reason:    "there is no per-format precompute to estimate",
	},
	{
		Path:      "pipeline.replay_match_page_size",
		RemovedIn: "P-6",
		Reason:    "the precompute replay is gone",
	},
	{
		Path:      "resources.precompute_mb_per_worker",
		RemovedIn: "P-6",
		Reason:    "see pipeline.precompute_concurrency",
	},
	{
		Path:      "resources.export_mb_per_worker",
		RemovedIn: "P-6",
		Reason:    "see pipeline.precompute_concurrency",
	},
	{
		Path:      "resources.seqcalc_mb_per_worker",
		RemovedIn: "P-6",
		Reason:    "see pipeline.precompute_concurrency",
	},
	{
		Path:      "resources.fielding_mb_per_worker",
		RemovedIn: "P-6",
		Reason:    "see pipeline.precompute_concurrency",
	},
	{
		Path:      "resources.seqcalc_low_memory_limit_gib",
		RemovedIn: "P-6",
		Reason:    "see pipeline.precompute_concurrency",
	},
	{
		Path:      "resources.precompute_concurrency_when_no_limit",
		RemovedIn: "P-6",
		Reason:    "see pipeline.precompute_concurrency",
	},
}

// RetiredKeys returns an error naming every retired setting the raw config carries.
//
// It reads the raw bytes rather than the decoded struct because that is the whole point:
// encoding/json drops a key with no field, so by the time there is a Config there is no
// evidence the key was ever there.
func RetiredKeys(raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil // a config that does not parse is the loader's problem, not this one's
	}
	found := make([]string, 0, len(retiredKeys))
	for _, key := range retiredKeys {
		if lookupPath(decoded, strings.Split(key.Path, ".")) {
			found = append(found, fmt.Sprintf("%s (removed in %s: %s)", key.Path, key.RemovedIn, key.Reason))
		}
	}
	if len(found) == 0 {
		return nil
	}
	sort.Strings(found)
	return fmt.Errorf(
		"config.json carries %d retired setting(s); remove them:\n  %s",
		len(found),
		strings.Join(found, "\n  "),
	)
}

// lookupPath reports whether the dotted path is present in the decoded config, at any
// value including null: a key written out as null is still someone stating an intention.
func lookupPath(node map[string]any, path []string) bool {
	for i, segment := range path {
		value, ok := node[segment]
		if !ok {
			return false
		}
		if i == len(path)-1 {
			return true
		}
		node, ok = value.(map[string]any)
		if !ok {
			return false
		}
	}
	return false
}
