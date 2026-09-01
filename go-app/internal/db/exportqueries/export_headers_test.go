package exportqueries_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	eq "github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-flow/go-app/internal/features"
)

// Why these tests exist
//
// Each export's SELECT order and its header list are maintained separately, and rows are
// read with scanx.ScanToStrings(rows, len(headers)). A change that drops a SELECT column
// without its header entry fails at scan time, loudly. A change that removes one column
// from the SELECT and a *different* one from the headers keeps the count equal and
// silently shifts every column in between -- an export that looks fine and is wrong.
//
// These assert the invariants that can be checked without a database. The strongest is
// TestInferenceHeadersMatchSharedContract: configs/feature_vectors.json is the contract
// ml-service reads too, so it is an independent source of truth rather than a snapshot
// of this package's own output.

// inferenceTrailer is the non-feature tail every inference export carries after its
// feature columns: the identifier and the fielding actuals used for evaluation.
var inferenceTrailer = []string{
	"player_name",
	"catches",
	"run_outs",
	"stumpings",
	"runouts_direct_hits",
	"fielding_involvements",
}

// Contract features the *_infer_*.csv exports deliberately omit.
//
// The cyclical time features are derived at prediction time rather than exported, and the
// sequence features are appended only when the sequence flag is on (AppendSeqIfEnabled).
// Live serving is unaffected: it goes through ComputeFeaturesAtCutoff*, whose
// ensureContractKeys fills every contract key, so the map cannot be missing one.
//
// This list is the point of the test. Add a feature to configs/feature_vectors.json
// without extending the export and the diff below stops matching, which is the reminder
// to decide whether the export needs it.
var inferenceOmitsFromContract = map[string][]string{
	"batting": {
		"match_month_sin", "match_month_cos", "match_day_of_week_sin", "match_day_of_week_cos",
		"bat_prev_sr", "bat_prev_out_rate", "bat_window_sr_12_pp", "bat_window_boundary_rate_12_pp",
		"bat_entry_sr_1_6", "bat_set_sr_13_30", "bat_react_after_dot_sr", "bat_after_k_dots_boundary_p_k2",
	},
	"bowling": {
		"match_month_sin", "match_month_cos", "match_day_of_week_sin", "match_day_of_week_cos",
		"bowl_prev_wkt_rate", "bowl_window_econ_24_death", "bowl_window_wkt_rate_24_death",
		"bowl_extras_wide_rate_pp", "bowl_react_after_boundary_wkt_rate_next",
		"bowl_spell_first_over_wkt_rate", "bowl_over_ball1_wkt_rate", "bowl_over_ball6_wkt_rate",
	},
}

func TestInferenceHeadersTrackSharedContract(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		headers  []string
		contract []string
	}{
		{name: "batting", headers: eq.BattingInferenceHeaders(), contract: features.BattingFeatureNames()},
		{name: "bowling", headers: eq.BowlingInferenceHeaders(), contract: features.BowlingFeatureNames()},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NotEmpty(t, tc.contract, "contract must not be empty")

			inExport := make(map[string]bool, len(tc.headers))
			for _, h := range tc.headers {
				inExport[h] = true
			}

			// 1. The set of contract features the export omits must be exactly the
			//    documented one -- so adding a contract feature fails here.
			var omitted []string
			var present []string
			for _, f := range tc.contract {
				if inExport[f] {
					present = append(present, f)
				} else {
					omitted = append(omitted, f)
				}
			}
			require.Equalf(t, inferenceOmitsFromContract[tc.name], omitted,
				"the set of contract features missing from the %s inference export changed.\n"+
					"If you added one to configs/feature_vectors.json, either export it or record it\n"+
					"in inferenceOmitsFromContract with the reason.", tc.name)

			// 2. The features it does export must appear in contract order. A same-count
			//    reordering is the one edit that would otherwise pass silently.
			var got []string
			for _, h := range tc.headers {
				for _, f := range tc.contract {
					if h == f {
						got = append(got, h)
						break
					}
				}
			}
			require.Equalf(t, present, got,
				"%s inference export columns are out of contract order.\n"+
					"Rows are read positionally, so order is part of the contract.", tc.name)

			// 3. Everything that is not a contract feature is the known trailer.
			var extra []string
			inContract := make(map[string]bool, len(tc.contract))
			for _, f := range tc.contract {
				inContract[f] = true
			}
			for _, h := range tc.headers {
				if !inContract[h] {
					extra = append(extra, h)
				}
			}
			require.Equalf(t, inferenceTrailer, extra,
				"%s inference export non-feature columns changed", tc.name)
		})
	}
}

func TestNoDuplicateHeaders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		headers []string
	}{
		{name: "batting_inference", headers: eq.BattingInferenceHeaders()},
		{name: "batting_format", headers: eq.BattingFormatHeaders()},
		{name: "batting_training", headers: eq.BattingTrainingHeaders()},
		{name: "bowling_inference", headers: eq.BowlingInferenceHeaders()},
		{name: "bowling_format", headers: eq.BowlingFormatHeaders()},
		{name: "bowling_training", headers: eq.BowlingTrainingHeaders()},
		{name: "extras_training", headers: eq.ExtrasTrainingHeaders()},
		{name: "innings_training", headers: eq.InningsTrainingHeaders()},
		{name: "fielding_training", headers: eq.FieldingTrainingHeaders()},
		{name: "fielding_holdout", headers: eq.FieldingHoldoutHeaders()},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			seen := make(map[string]int, len(tc.headers))
			for idx, h := range tc.headers {
				require.NotEmpty(t, strings.TrimSpace(h), "empty column name at index %d", idx)
				if first, dup := seen[h]; dup {
					t.Fatalf("duplicate column %q at indices %d and %d: a duplicate means two "+
						"SELECT expressions land in the same named column downstream", h, first, idx)
				}
				seen[h] = idx
			}
		})
	}
}

// The per-format and cross-format training exports feed ml/train_batting.py and
// ml/train_bowling.py, which reference columns by name. The cross-format export is the
// per-format one plus its extra label and match_date, so drift between the two shows up
// as a column present in one and missing from the other.
func TestTrainingExportsAgreeWithEachOther(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		perFormat []string
		crossFmt  []string
		extraOnly []string // columns the cross-format export adds
	}{
		{
			name:      "batting",
			perFormat: eq.BattingFormatHeaders(),
			crossFmt:  eq.BattingTrainingHeaders(),
			// The four cyclical columns are computed from match_date by the cross-format
			// row builder; the per-format export does not emit them at all. Pre-existing
			// gap, tracked separately -- adding names there without values would misalign it.
			extraOnly: []string{
				"innings_runs",
				"match_date",
				"match_month_sin",
				"match_month_cos",
				"match_day_of_week_sin",
				"match_day_of_week_cos",
			},
		},
		{
			name:      "bowling",
			perFormat: eq.BowlingFormatHeaders(),
			crossFmt:  eq.BowlingTrainingHeaders(),
			// the bowling cross-format export carries both innings totals
			extraOnly: []string{
				"innings_runs",
				"innings_wickets",
				"match_date",
				"match_month_sin",
				"match_month_cos",
				"match_day_of_week_sin",
				"match_day_of_week_cos",
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cross := make(map[string]bool, len(tc.crossFmt))
			for _, h := range tc.crossFmt {
				cross[h] = true
			}
			var missing []string
			for _, h := range tc.perFormat {
				if !cross[h] {
					missing = append(missing, h)
				}
			}
			require.Emptyf(t, missing,
				"columns in the per-format %s export but not the cross-format one: %v.\n"+
					"Both feed the same trainer, so a column must be added to or removed from both.",
				tc.name, missing)

			perFmt := make(map[string]bool, len(tc.perFormat))
			for _, h := range tc.perFormat {
				perFmt[h] = true
			}
			allowed := make(map[string]bool, len(tc.extraOnly))
			for _, h := range tc.extraOnly {
				allowed[h] = true
			}
			var unexpected []string
			for _, h := range tc.crossFmt {
				if !perFmt[h] && !allowed[h] {
					unexpected = append(unexpected, h)
				}
			}
			require.Emptyf(t, unexpected,
				"columns in the cross-format %s export but not the per-format one: %v.\n"+
					"If this is intentional, add them to extraOnly in this test.",
				tc.name, unexpected)
		})
	}
}

// A guard against the specific failure mode that makes this package risky to edit:
// counts drifting apart. The numbers are deliberately hard-coded so that changing an
// export requires acknowledging the change here.
func TestHeaderCountsAreStable(t *testing.T) {
	t.Parallel()

	// Counts dropped by 7 across every export in C2-2b, when the weather features were
	// removed from configs/feature_vectors.json and the export queries together.
	//
	// The fielding counts are 16 and the two training counts 40, not 12 and 36: every
	// one of those row builders emits the four cyclical time columns added in v3, and
	// the header lists were never widened to name them. That is what shipped fielding
	// CSVs with 12 headers over 16-field rows, and what made the batting and bowling
	// sections of the training-data API hand back rows four fields too wide.
	want := map[string]int{
		"batting_inference": 29,
		"batting_format":    34,
		"batting_training":  40,
		"bowling_inference": 29,
		"bowling_format":    33,
		"bowling_training":  40,
		"extras_training":   9,
		"innings_training":  12,
		"fielding_training": 16,
		"fielding_holdout":  16,
	}
	got := map[string]int{
		"batting_inference": len(eq.BattingInferenceHeaders()),
		"batting_format":    len(eq.BattingFormatHeaders()),
		"batting_training":  len(eq.BattingTrainingHeaders()),
		"bowling_inference": len(eq.BowlingInferenceHeaders()),
		"bowling_format":    len(eq.BowlingFormatHeaders()),
		"bowling_training":  len(eq.BowlingTrainingHeaders()),
		"extras_training":   len(eq.ExtrasTrainingHeaders()),
		"innings_training":  len(eq.InningsTrainingHeaders()),
		"fielding_training": len(eq.FieldingTrainingHeaders()),
		"fielding_holdout":  len(eq.FieldingHoldoutHeaders()),
	}

	for name, w := range want {
		require.Equalf(t, w, got[name],
			"%s column count changed (%d -> %d).\n"+
				"Update this expectation *and* confirm the query's SELECT changed by the same amount --\n"+
				"scanx.ScanToStrings uses len(headers), so the two must move together.",
			name, w, got[name])
	}
	require.Len(t, got, len(want), "an export was added or removed without updating this test")
}

// Documents the naming split that surprises people reading these exports: the inference
// path uses the prefixed names from configs/feature_vectors.json ("batting_temp",
// "venue"), while the training CSVs use unprefixed ones ("temp", "batting_venue") to
// match ml/train_batting.py:FEATURE_COLS. Both are deliberate; this pins them so a
// well-meaning "consistency" edit to one side fails here rather than in training.
func TestInferenceAndTrainingUseDifferentNamesDeliberately(t *testing.T) {
	t.Parallel()

	inference := eq.BattingInferenceHeaders()
	training := eq.BattingFormatHeaders()

	has := func(hs []string, name string) bool {
		for _, h := range hs {
			if h == name {
				return true
			}
		}
		return false
	}

	for _, pair := range []struct{ inferenceName, trainingName string }{
		{inferenceName: "batting_inning", trainingName: "inning"},
		{inferenceName: "venue", trainingName: "batting_venue"},
		{inferenceName: "opposition", trainingName: "batting_opposition"},
	} {
		require.Truef(t, has(inference, pair.inferenceName),
			"inference export lost %q", pair.inferenceName)
		require.Truef(t, has(training, pair.trainingName),
			"training export lost %q", pair.trainingName)
	}
}
