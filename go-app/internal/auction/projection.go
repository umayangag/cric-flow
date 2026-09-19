package auction

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

// The projection's assumptions and the arithmetic over its answers (P3-2).
//
// A projection is conditional on three things the operator names and none of them is a
// fact: the eleven the candidate would join, the opposition that eleven would face, and
// the grounds. At an auction all three are guesses, so every one of them is held on the
// record, carried back on every answer and shown on the surface as an assumption. A
// projection against a silently neutral side would be a projection for no league (§8.7).
//
// Nothing here selects, ranks or scores anything. The rule the module is built on is that
// this is valuation and projection and never XI-picking: the record is that optimised
// selection in domestic T20 is indistinguishable from rating order (plan §8.8), the IPL is
// domestic T20, and so no path from this package reaches the objective.

// TeamSize is what an eleven is. The projection refuses anything else for the same reason
// the predict path does: every number on the answer aggregates one side into one row, so a
// ten-man side would be scored as a match nobody plays and would look like every other
// answer (P1-2, §8.7).
const TeamSize = 11

// The interval sources a projected number can carry (P3-2).
//
// Two intervals sit side by side on the surface and they are different populations with
// different evidence behind them: L2-B's quantile heads are at nominal coverage on the
// harness (plan §8.2), and the simulator's draws are B-11's open defect. A reader who
// could not tell them apart would read one's evidence onto the other, so each interval
// names its source and opens its own explainer.
//
// Wire vocabulary, declared once in contracts/ops-console.contract.json and asserted from
// every side that reads it (H-24).
const (
	IntervalSourceL2BQuantiles   = "l2b_quantiles"
	IntervalSourceSimulatorDraws = "simulator_draws"
)

// IntervalSources returns every source an interval on the projection may name.
func IntervalSources() []string {
	return []string{IntervalSourceL2BQuantiles, IntervalSourceSimulatorDraws}
}

// The L-1 keys the projection labels its numbers under, beside P3-1's two.
//
// `spread_share` is deliberately not a new key: the Lab already explains it and the
// auction shows the same quantity, so it reuses that entry rather than minting a second
// explanation of one number.
const (
	MetricProjectedOutput   = "auction_projected_output"
	MetricProjectedTotal    = "auction_projected_total"
	MetricIntervalL2B       = "interval_source_l2b_quantiles"
	MetricIntervalSimulator = "interval_source_simulator_draws"
	MetricProjectedSpread   = "spread_share"
)

// NamedPlayer is one player an assumption names: the record's id, the registry id
// ml-service knows him by, and his name for the surface.
//
// The registry id is carried because a player the database holds and the registry does not
// cannot be sent to the model at all, and the projection says so by name rather than
// quietly leaving him out of the eleven it scores.
type NamedPlayer struct {
	PlayerID   int64
	ExternalID string
	PlayerName string
}

// Opposition is the eleven the operator says the projection is against, and the side it
// stands for.
//
// The club id is not decoration. The performance model reads a ground only through the
// team context, and `ml.xi.rows.team_context_or_neutral` falls back to neutral for the
// *pair* — venue included — whenever either side is unnamed. So an opposition with no club
// id would make every ground read the same, and the venue mix this module exists for would
// be three identical rows.
type Opposition struct {
	OppositionID int64
	Name         string
	Players      []NamedPlayer
}

// ErrNoLikelyEleven and ErrNoOpposition report an assumption the record does not hold yet.
// Named rather than folded into the incomplete-eleven error because the operator's next
// action is different: one is "name the eleven", the other is "name who it plays".
var (
	ErrNoLikelyEleven = errors.New(
		"this auction has no likely eleven; a projection is for an eleven the operator names",
	)
	ErrNoOpposition = errors.New(
		"this auction has no opposition; a projection against no one is a projection for no league",
	)
	ErrNoGrounds = errors.New("this auction names no grounds; a projection is per ground")
)

// IncompleteElevenError reports a side that is not an eleven, in the terms the operator
// can act on: how many the eleven would hold with the candidate in it, and how many it
// needs.
type IncompleteElevenError struct {
	Have int
	Need int
}

func (e *IncompleteElevenError) Error() string {
	return fmt.Sprintf(
		"the likely eleven with this candidate in it holds %d players, and an eleven is scored as %d",
		e.Have, e.Need)
}

// UnregisteredPlayerError reports players an assumption names that ml-service cannot be
// asked about, because this database holds no registry id for them.
//
// Refused, not dropped: scoring the ten who resolved would answer for an eleven nobody
// named, and nothing on the answer would say so (§8.7).
type UnregisteredPlayerError struct {
	Where     string
	PlayerIDs []int64
}

func (e *UnregisteredPlayerError) Error() string {
	ids := make([]string, 0, len(e.PlayerIDs))
	for _, id := range e.PlayerIDs {
		ids = append(ids, fmt.Sprint(id))
	}
	return fmt.Sprintf(
		"%s: player ids %s carry no registry id, so the served ratings cannot be asked about them",
		e.Where, strings.Join(ids, ", "))
}

// ProjectedEleven is the side the candidate would be projected in: the likely eleven with
// him added, and him left where he already is if the operator has him in it.
//
// Deduplicating rather than appending blindly is what lets one rule cover both ways an
// operator works. A likely eleven of ten with an open place takes the candidate as its
// eleventh; a likely eleven of eleven that already names him is projected as it stands.
// Anything else is refused — a ten-man side, and equally a full eleven plus an outside
// candidate, which is twelve men and not a decision anyone can read.
func ProjectedEleven(likelyXI []NamedPlayer, candidate NamedPlayer) ([]NamedPlayer, error) {
	eleven := make([]NamedPlayer, 0, len(likelyXI)+1)
	eleven = append(eleven, likelyXI...)
	if !holdsPlayer(likelyXI, candidate.PlayerID) {
		eleven = append(eleven, candidate)
	}
	if len(eleven) != TeamSize {
		return nil, &IncompleteElevenError{Have: len(eleven), Need: TeamSize}
	}
	return eleven, nil
}

// SharedPlayerError reports an assumption that puts one player on both sides of the
// projected match.
//
// Both elevens are the operator's own: the likely eleven he named, the opposition he named,
// and the candidate he is valuing. Where they overlap, the answer would be a match in which
// one man fields for both teams -- and the projected output would be his own contribution
// to the side he is projected against. Refused rather than resolved: this module has no
// evidence to pick a side with (an auction is precisely the moment a player has no club),
// and silently dropping him from one eleven would answer for an eleven nobody named (§8.7).
type SharedPlayerError struct {
	Players []NamedPlayer
}

func (e *SharedPlayerError) Error() string {
	names := make([]string, 0, len(e.Players))
	for _, player := range e.Players {
		names = append(names, fmt.Sprintf("%s (id %d)", player.PlayerName, player.PlayerID))
	}
	return fmt.Sprintf(
		"the likely eleven and the opposition both name %s, and nobody plays both sides; "+
			"take him out of one of them", strings.Join(names, ", "))
}

// RefuseSharedPlayers refuses a projection whose two elevens share a player.
func RefuseSharedPlayers(eleven, opposition []NamedPlayer) error {
	var shared []NamedPlayer
	for _, player := range eleven {
		if holdsPlayer(opposition, player.PlayerID) {
			shared = append(shared, player)
		}
	}
	if len(shared) == 0 {
		return nil
	}
	return &SharedPlayerError{Players: shared}
}

func holdsPlayer(players []NamedPlayer, playerID int64) bool {
	for _, player := range players {
		if player.PlayerID == playerID {
			return true
		}
	}
	return false
}

// RegistryKeys returns the registry ids ml-service scores a side by, refusing the side
// where any member has none.
func RegistryKeys(players []NamedPlayer, where string) ([]string, error) {
	keys := make([]string, 0, len(players))
	var missing []int64
	for _, player := range players {
		if player.ExternalID == "" {
			missing = append(missing, player.PlayerID)
			continue
		}
		keys = append(keys, player.ExternalID)
	}
	if len(missing) > 0 {
		return nil, &UnregisteredPlayerError{Where: where, PlayerIDs: missing}
	}
	return keys, nil
}

// Quantiles is a 10-50-90 summary of one quantity.
type Quantiles struct {
	Q10    float64
	Median float64
	Q90    float64
}

// MixtureComponent is one ground's drawn totals and the weight the operator gave that
// ground in the mix.
type MixtureComponent struct {
	Draws  []float64
	Weight float64
}

// ErrEmptyMixture reports a mixture with nothing in it to pool.
var ErrEmptyMixture = errors.New("a mixture over grounds needs at least one ground with draws and a positive weight")

// MixtureQuantiles is the 10-50-90 of the eleven's total over the operator's venue mix,
// computed from the pooled draws.
//
// **Why the draws and not the quantiles.** A mixture's quantiles are not the mean of its
// parts' quantiles — averaging three q90s gives a number no distribution has — so the
// three per-ground summaries cannot be combined at all. What can be combined is the draws
// behind them: each ground's draw carries mass `weight / n` in the pool, and the pooled
// empirical distribution is inverted directly. That is why `/simulate` is asked for
// `return_total_draws` at all.
//
// The inversion is the plain one: the p-quantile is the smallest drawn total whose
// cumulative mass reaches p. No interpolation between draws, because an interpolated point
// is a total no simulation produced, and this module shows what was drawn.
func MixtureQuantiles(components []MixtureComponent) (Quantiles, error) {
	type weighted struct {
		value float64
		mass  float64
	}
	pooled := make([]weighted, 0)
	totalWeight := 0.0
	for _, component := range components {
		if component.Weight <= 0 || len(component.Draws) == 0 {
			continue
		}
		totalWeight += component.Weight
		mass := component.Weight / float64(len(component.Draws))
		for _, draw := range component.Draws {
			pooled = append(pooled, weighted{value: draw, mass: mass})
		}
	}
	if len(pooled) == 0 || totalWeight <= 0 {
		return Quantiles{}, ErrEmptyMixture
	}
	sort.Slice(pooled, func(i, j int) bool { return pooled[i].value < pooled[j].value })

	quantileAt := func(p float64) float64 {
		target := p * totalWeight
		cumulative := 0.0
		for _, entry := range pooled {
			cumulative += entry.mass
			// The float comparison is loosened by one part in 10^9 of the total mass so the
			// last draw is reachable at p = 1: exact arithmetic would have the sum land a
			// few ulps below the target and walk off the end of the pool.
			if cumulative >= target-math.Abs(totalWeight)*1e-9 {
				return entry.value
			}
		}
		return pooled[len(pooled)-1].value
	}
	return Quantiles{Q10: quantileAt(0.10), Median: quantileAt(0.50), Q90: quantileAt(0.90)}, nil
}
