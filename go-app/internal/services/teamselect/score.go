package teamselect

// Package teamselect contains pure helpers to score players for selection.
// These helpers are deterministic and side-effect-free to enable high coverage tests.

// Player is a simplified model used by scoring/selection services.
// Keep only fields required for pure scoring to avoid coupling to DB models.
type Player struct {
	Name      string
	IsBowler  bool
	IsKeeper  bool
	BatScore  float64 // normalized batting signal
	BowlScore float64 // normalized bowling signal
}

// ScoreWeights defines relative weights for combining signals into a single score.
type ScoreWeights struct {
	Bat         float64
	Bowl        float64
	KeeperBonus float64 // extra additive bonus if the player can keep wickets
}

// DefaultWeights returns a conservative weighting leaning slightly toward batting.
func DefaultWeights() ScoreWeights { return ScoreWeights{Bat: 0.55, Bowl: 0.45, KeeperBonus: 0.02} }

// ScorePlayer computes a scalar score for a player using the provided weights.
// It applies a keeper bonus when applicable and normalizes obvious bounds.
func ScorePlayer(p Player, w ScoreWeights) float64 {
	bat := clamp01(p.BatScore)
	bowl := clamp01(p.BowlScore)
	s := w.Bat*bat + w.Bowl*bowl
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
