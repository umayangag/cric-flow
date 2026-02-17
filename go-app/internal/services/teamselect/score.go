package teamselect

// Package teamselect contains pure helpers to score players for selection.
// These helpers are deterministic and side-effect-free to enable high coverage tests.

// Player is a simplified model used by scoring/selection services.
// Keep only fields required for pure scoring to avoid coupling to DB models.
type Player struct {
	Name       string
	IsBowler   bool
	IsKeeper   bool
	BatScore   float64 // normalized batting signal (e.g. predicted runs contribution)
	BowlScore  float64 // normalized bowling signal (e.g. wickets/economy contribution)
	FieldScore float64 // normalized fielding signal (catches, run_outs; 0 if not set)
}

// ScoreWeights defines relative weights for combining signals into a single score.
type ScoreWeights struct {
	Bat         float64
	Bowl        float64
	Field       float64 // weight for fielding (0 = ignore)
	KeeperBonus float64 // extra additive bonus if the player can keep wickets
}

// DefaultWeights returns a conservative weighting: batting, bowling, fielding, keeper bonus.
func DefaultWeights() ScoreWeights {
	return ScoreWeights{Bat: 0.45, Bowl: 0.40, Field: 0.10, KeeperBonus: 0.02}
}

// ScorePlayer computes a scalar score for a player using the provided weights.
// It applies a keeper bonus when applicable and normalizes obvious bounds.
func ScorePlayer(p Player, w ScoreWeights) float64 {
	bat := clamp01(p.BatScore)
	bowl := clamp01(p.BowlScore)
	s := w.Bat*bat + w.Bowl*bowl
	if w.Field > 0 {
		s += w.Field * clamp01(p.FieldScore)
	}
	if p.IsKeeper {
		s += w.KeeperBonus
	}
	return s
}

// clamp01 limits x into the inclusive range [0,1].
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
