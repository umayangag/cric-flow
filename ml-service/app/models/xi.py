"""Request / response models for the XI-responsive win endpoints (``/xi/*``), the
player-performance endpoint (``/performance/predict``) and the match simulator
(``/simulate``).

Players are identified by id only. The ML service holds the as-of rating state, so callers
send who is playing, not what their features are -- which is also what makes the training
and serving paths compute the same function of the same eleven names (S-3c).

**The id is the Cricsheet registry identifier** (``player.external_id`` in the go-app
database, ``info.registry.people`` in the archive) -- the key the rating state is built on
since P-1, and the one thing that means the same player on both sources. It is a string:
these ids are hex (``2911de16``). They were typed as ``int`` until P-5, which no id the
store holds could satisfy, so every request named eleven players the state had never seen
and every prediction was computed for debutants.
"""

from __future__ import annotations

from datetime import date
from typing import Dict, List, Literal, Optional

from pydantic import BaseModel, Field, field_validator


def _upper(v: Optional[str]) -> Optional[str]:
    return v if v is None or v == "" else v.strip().upper()


class XiConstraints(BaseModel):
    team_size: int = Field(default=11, ge=1, le=15)
    min_bowlers: int = Field(default=5, ge=0, le=11)
    require_keeper: bool = True
    must_include: List[str] = Field(default_factory=list)
    must_exclude: List[str] = Field(default_factory=list)


class XiOptimizeRequest(BaseModel):
    format: str
    pool_player_ids: List[str] = Field(..., min_length=1)
    opponent_player_ids: List[str] = Field(
        default_factory=list,
        description="The opposing XI the selection is made against; not read by objective='ratings'",
    )
    objective: Literal["win", "ratings"] = Field(
        default="win",
        description="'win' searches for the XI that maximises the objective model's P(win); "
        "'ratings' returns the rating-ordered pick and evaluates no model, which is the only "
        "mode offered where the objective does not rank (H-17, TEST)",
    )
    team_is_team1: bool = Field(default=True, description="Whether the pool's side bats first")
    constraints: XiConstraints = Field(default_factory=XiConstraints)
    max_evaluations: int = Field(default=20000, ge=100, le=200000)
    as_of: Optional[date] = Field(
        default=None,
        description="Backtests only: serve from ratings as they stood before this date "
        "instead of through today. Omit for live predictions.",
    )

    @field_validator("format", mode="before")
    def _format_upper(cls, v: str) -> str:
        return _upper(v) or ""


class ServedRatings(BaseModel):
    """Which rating state answered: the run it was loaded from and the date its ratings run
    through (P1-5).

    It rides on every prediction response rather than being read off ``/xi/status``
    afterwards, because a status read describes whatever is loaded *now*, and a reload can
    land between a prediction and the read. Stamped here, the date is the one the numbers
    beside it were computed from -- the same ``state.last_date`` and manifest ``/xi/status``
    reports, so the two cannot disagree about a store, only about which store."""

    run_id: str = Field(..., description="The run the served models and rating state were loaded from")
    ratings_through: str = Field(..., description="YYYY-MM-DD: the last match date the served ratings include")


class XiOptimizeResponse(BaseModel):
    selected_player_ids: List[str]
    objective: Literal["win", "ratings"]
    optimised: bool = Field(
        ...,
        description="False for the rating-ordered pick: a selection, but not one that maximises "
        "anything. Callers must say so (H-17)",
    )
    win_probability: Optional[float] = Field(
        default=None, ge=0, le=1, description="None when nothing was maximised (objective='ratings')"
    )
    evaluations: int
    improved_over_seed: float
    unknown_player_ids: List[str] = Field(
        default_factory=list, description="Pool ids with no rating history; treated as debutants"
    )
    marginal_values: Dict[str, float] = Field(
        default_factory=dict, description="P(win) lost if the player were replaced by an average one"
    )
    served_ratings: ServedRatings


class XiWinRequest(BaseModel):
    format: str
    team1_player_ids: List[str] = Field(..., min_length=1)
    team2_player_ids: List[str] = Field(..., min_length=1)
    team1_id: Optional[int] = Field(
        default=None, description="opposition id of the side batting first (for team-level context)"
    )
    team2_id: Optional[int] = None
    venue_id: Optional[int] = None
    team1_bats_first: Optional[bool] = Field(
        default=None, description="Known after the toss; omit before it to average both batting orders"
    )
    team1_constraints: Optional[XiConstraints] = Field(
        default=None,
        description="Play mode (P1-2): check this eleven against these constraints instead of selecting "
        "under them. Omitted where the eleven came from the optimiser, which applied them while searching.",
    )
    team2_constraints: Optional[XiConstraints] = Field(default=None, description="The same, for team2's eleven")
    as_of: Optional[date] = Field(
        default=None,
        description="Backtests only: serve from ratings as they stood before this date "
        "instead of through today. Omit for live predictions.",
    )

    @field_validator("format", mode="before")
    def _format_upper(cls, v: str) -> str:
        return _upper(v) or ""


class XiConstraintCheck(BaseModel):
    """Whether an eleven somebody *built* meets the constraints it was sent with (P1-2).

    The optimiser applies these constraints while it searches; a hand-built eleven is
    never searched, so nothing applies them to it -- and repairing it silently would be
    scoring a different eleven from the one on screen. So it is checked and reported
    instead: the counts are the ones the constraint is defined on, not a caller's guess
    at them. ``is_bowling_option`` is the same definition the optimiser and the feature
    share (``ml.xi.contract``), and the keeper flag is the served rating state's, so
    "five bowlers" means here exactly what it means inside the search.

    Every field is on the wire rather than just ``met``: a chip that says "4 of 5
    bowlers" is a broken constraint a user can act on, and "not met" is not.
    """

    team_size: int = Field(..., description="How many players the eleven actually holds")
    bowlers: int = Field(..., description="How many of them are bowling options, by the optimiser's definition")
    min_bowlers: int = Field(..., description="The minimum asked for, echoed so the answer stands alone")
    has_keeper: bool
    require_keeper: bool
    missing_must_include: List[str] = Field(
        default_factory=list, description="must_include ids the eleven does not hold"
    )
    met: bool = Field(..., description="True where the eleven satisfies every constraint above")


class XiWinResponse(BaseModel):
    team1_win_probability: float = Field(..., ge=0, le=1)
    objective_probability: float = Field(
        ..., ge=0, le=1, description="XI-only model, the value the optimiser maximises"
    )
    team1_constraint_check: Optional[XiConstraintCheck] = Field(
        default=None, description="Present only where the request carried team1_constraints"
    )
    team2_constraint_check: Optional[XiConstraintCheck] = Field(
        default=None, description="Present only where the request carried team2_constraints"
    )
    served_ratings: ServedRatings


class RatingsFreshness(BaseModel):
    """H-11's verdict, not just the date.

    ``/xi/status`` used to report ``ratings_through`` and leave the reader to work out
    whether that was recent enough. The rule is a number in config, so the service is the
    one that should apply it -- and the same verdict is what a prediction request is
    refused on, so there is one answer rather than two that can disagree."""

    fresh: bool
    age_days: Optional[int] = None
    max_age_days: int
    ratings_through: Optional[str] = None
    code: Optional[str] = Field(
        default=None,
        description="RATINGS_STALE when a live prediction would be refused; absent when it would not",
    )


class XiStatusResponse(BaseModel):
    loaded: bool
    formats: List[str]
    performance_formats: List[str] = Field(default_factory=list)
    players: int
    ratings_through: Optional[str]
    report: Optional[dict] = None
    # Which run these artifacts came from and what it recorded about itself (H-16).
    # None when nothing is loaded, or when what is on disk was refused.
    run_id: Optional[str] = None
    manifest: Optional[dict] = None
    # Why nothing is loaded, when something is on disk but could not be served (D-6).
    # An empty panel and a refused artifact set look the same otherwise, and only one of
    # them is something an operator has to act on.
    error: Optional[str] = None
    ratings: Optional[RatingsFreshness] = None


class PerformancePredictRequest(XiWinRequest):
    """The same inputs as a win prediction: both elevens by id, the format, optional team
    and venue ids for context, the toss once known, and ``as_of`` for backtests."""


class PerformanceRange(BaseModel):
    q10: float
    median: float
    q90: float


class WicketDistribution(BaseModel):
    expected: float = Field(..., description="Mean of the count distribution")
    p0: float = Field(..., ge=0, le=1)
    p1: float = Field(..., ge=0, le=1)
    p2_plus: float = Field(..., ge=0, le=1)


class PlayerPerformance(BaseModel):
    player_id: str
    side: int = Field(..., description="1 = team1, 2 = team2")
    p_bats: float = Field(..., ge=0, le=1)
    p_bowls: float = Field(..., ge=0, le=1)
    runs: PerformanceRange
    balls_faced: PerformanceRange
    runs_conceded: PerformanceRange
    wickets: WicketDistribution
    catches_expected: float = Field(..., description="Poisson rate; reported, never a headline")


class PerformancePredictResponse(BaseModel):
    players: List[PlayerPerformance]
    innings_marginalised: bool = Field(
        ..., description="True when the toss was unknown and both batting orders were averaged"
    )
    unknown_player_ids: List[str] = Field(
        default_factory=list, description="Ids with no rating history; predicted as debutants"
    )
    served_ratings: ServedRatings


class SimulateRequest(PerformancePredictRequest):
    """A performance prediction's inputs plus the draw count and seed. The simulator (L2-C)
    runs only for formats with an innings length (T20, T20I, ODI)."""

    n_samples: int = Field(default=2000, ge=100, le=20000, description="Draws; the served default is 2000")
    seed: int = Field(default=0, ge=0, description="Draws are deterministic given the inputs and the seed")


class SimulatedScorecardLine(BaseModel):
    """One player's line of the median-band scorecard: the mean over the draws whose side
    total lies in the central tenth of its distribution. The lines plus extras sum to the
    side's ``total.scorecard`` by construction."""

    runs: float
    balls_faced: float
    wickets: float
    runs_conceded: float
    balls_bowled: float


class SimulatedPlayer(BaseModel):
    player_id: str
    side: int = Field(..., description="1 = team1, 2 = team2")
    p_bats: float = Field(..., ge=0, le=1, description="Share of draws in which the player batted")
    p_bowls: float = Field(..., ge=0, le=1)
    runs: PerformanceRange
    balls_faced: PerformanceRange
    wickets: PerformanceRange
    runs_conceded: PerformanceRange
    balls_bowled: PerformanceRange
    scorecard: SimulatedScorecardLine
    spread_share: float = Field(..., description="Cov(player runs, side total) / Var(side total); shares sum to 1")
    spread_runs: float = Field(..., description="spread_share times the side total's standard deviation")


class SimulatedTotal(PerformanceRange):
    mean: float
    sd: float
    scorecard: float = Field(..., description="Mean total over the median band; what the scorecard lines sum to")


class SimulatedSide(BaseModel):
    total: SimulatedTotal
    extras_scorecard: float = Field(..., description="Extras in the median-band scorecard")
    extras_spread_share: float
    wickets_lost: PerformanceRange
    players: List[SimulatedPlayer]


class SimulatedMargin(BaseModel):
    """The margin as cricket states it: runs when the side batting first wins, balls
    remaining and wickets in hand when the chaser does."""

    p_bat_first_wins: float
    p_chaser_wins: float
    p_tie: float
    runs_when_bat_first_wins: Optional[PerformanceRange] = None
    balls_remaining_when_chaser_wins: Optional[PerformanceRange] = None
    wickets_in_hand_when_chaser_wins: Optional[PerformanceRange] = None


class SimulatedWinProbability(BaseModel):
    simulated: float = Field(..., ge=0, le=1, description="P(team1 wins) by simulation, a tie counted half")
    p_tie: float = Field(..., ge=0, le=1)
    display: float = Field(..., ge=0, le=1, description="The display model's P(team1 wins) for the same fixture")
    headline: float = Field(..., ge=0, le=1, description="The probability to show, per E2's rule")
    headline_source: str = Field(..., description="'display' or 'simulator' (plan §5, E2)")


class SimulateResponse(BaseModel):
    format: str
    n_samples: int
    seed: int
    toss_marginalised: bool = Field(..., description="True when the toss was unknown and half the draws went each way")
    team1: SimulatedSide
    team2: SimulatedSide
    win_probability: SimulatedWinProbability
    margin: SimulatedMargin
    unknown_player_ids: List[str] = Field(default_factory=list)
    served_ratings: ServedRatings
