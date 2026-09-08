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


class BestAlternativeModel(BaseModel):
    """The pool player the objective would most like instead of a selected one, and the
    P(win) that swap costs (P1-3).

    A point estimate with no interval, deliberately: it is one evaluation of the objective
    on the swap candidates the search itself scores, and nothing in that computation
    produces an uncertainty. A gap at or below zero means the search stopped on its
    evaluation budget rather than at a local optimum."""

    player_id: str = Field(..., description="Registry id of the best excluded alternative from the same pool")
    win_probability_gap: float = Field(
        ..., description="P(win) with the selected player minus P(win) with this alternative in his place"
    )


class PlayerSelectionReasonModel(BaseModel):
    """Why one selected player is in the eleven, in the terms the selection used (P1-3).

    Only what the selection consumed: the constraint predicates it evaluated, the composite
    it ordered the pool by, and the swap candidates it scored. Nothing is a second opinion
    about the player."""

    roles: List[str] = Field(
        default_factory=list,
        description="Constraint state this player answers: 'keeper', 'bowling_option' (ml.xi.roles.SELECTION_ROLES)",
    )
    selection_rating: float = Field(
        ..., description="The composite the pool was ordered by (ml.xi.optimizer.rating_order_score)"
    )
    rating_percentile: float = Field(
        ..., ge=0, le=100, description="Share of this pool the player outranks on that composite, 0-100"
    )
    pool_size: int = Field(..., description="How many candidates the percentile is taken over")
    best_alternative: Optional[BestAlternativeModel] = Field(
        default=None, description="Absent on the rating-ordered path: nothing was maximised, so nothing was compared"
    )
    best_alternative_note: Optional[str] = Field(
        default=None, description="Why the objective could name no alternative, where it was asked and could not"
    )


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
    selection_reasons: Dict[str, PlayerSelectionReasonModel] = Field(
        default_factory=dict,
        description="Per selected player, what the selection read about him: role, rating standing in the "
        "pool, and -- where an objective was maximised -- the best excluded alternative (P1-3)",
    )
    served_ratings: ServedRatings


class PlayerRolesRequest(BaseModel):
    """Ask what the served vectors say about a list of players (P3-1).

    A list of ids and nothing else: no eleven, no opponent, no constraints. The auction
    module asks this about players who are in no eleven at all -- which is precisely why
    it is a route of its own rather than a corner of ``/xi/optimize``. Nothing here
    selects, ranks or scores, and no answer of this route depends on the objective model.
    """

    format: str
    player_ids: List[str] = Field(..., min_length=1, max_length=500)

    @field_validator("format", mode="before")
    def _format_upper(cls, v: str) -> str:
        return _upper(v) or ""


class PlayerRoles(BaseModel):
    """What the served as-of vectors say about one player.

    ``known`` is false where the served rating state has never seen the id. His ``roles``
    are then empty and mean nothing: no role is invented for a player the model has never
    read, and the caller must show him as unknown rather than as a batter (§8.7). "The
    model says he neither keeps nor bowls" and "the model has never seen him" are
    different answers.
    """

    player_id: str
    known: bool = Field(..., description="False where the served rating state holds no vectors for this id")
    roles: List[str] = Field(
        default_factory=list,
        description="Subset of ml.xi.roles.SELECTION_ROLES: 'keeper', 'bowling_option'. Empty for an "
        "unknown player, and -- for a known one -- a batter by elimination",
    )


class PlayerRolesResponse(BaseModel):
    """The roles for every id asked for, in the order they were asked for."""

    format: str
    players: List[PlayerRoles]
    unknown_player_ids: List[str] = Field(
        default_factory=list, description="Ids the served rating state has never seen, named rather than dropped"
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
    at them. Both predicates come from ``ml.xi.roles`` -- the one definition the optimiser,
    this check and the auction module's role read all share -- over the served rating
    state's own vectors, so "five bowlers" means here exactly what it means inside the search.

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
    # The last reload's refusal (D-6), naming the run it refused. With ``loaded`` false it
    # is why nothing is serving -- an empty panel and a refused artifact set look the same
    # otherwise, and only one of them is something an operator has to act on. With
    # ``loaded`` true, a reload named a run that could not be served and ``run_id`` went
    # on serving (B-13): the two fields describe two different runs.
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


class VenueContext(BaseModel):
    """What the served rating state knows about the ground these rows were built with --
    read off the rows the model consumed, never recomputed (P3-2).

    The performance model reads a ground through exactly two columns
    (``contract.performance_feature_cols``): its bat-first rate and the sample size behind
    it. The ground's *scoring level* was gated and recorded as a null (A-1, plan §8.9) and
    is not consumed -- ``FIXTURE_CONTEXT_FAMILIES_KEPT`` is empty. So a caller comparing
    two grounds is comparing what the toss does at each, and nothing else about them.

    ``neutral`` is the honest statement §8.7 asks for: a ground the state has no matches
    for -- and equally a request that named no teams, since ``rows.team_context_or_neutral``
    falls back for the pair, not for the venue alone -- is read at the prior, 0.5 with n 0.
    A caller that showed such a row as "at the Wankhede" without saying so would be showing
    a substitution nobody could see."""

    venue_bf_rate: float = Field(
        ..., description="The ground's bat-first win rate, shrunk toward 0.5 over VENUE_PRIOR_MATCHES"
    )
    venue_n: float = Field(..., description="How many matches at this ground the served state folded in")
    neutral: bool = Field(
        ..., description="True where venue_n is 0: the ground contributed nothing and the rows read at the prior"
    )


class PerformancePredictResponse(BaseModel):
    players: List[PlayerPerformance]
    innings_marginalised: bool = Field(
        ..., description="True when the toss was unknown and both batting orders were averaged"
    )
    unknown_player_ids: List[str] = Field(
        default_factory=list, description="Ids with no rating history; predicted as debutants"
    )
    venue_context: VenueContext = Field(
        ..., description="What the served state knows about the ground these rows were built with (P3-2)"
    )
    served_ratings: ServedRatings


class SimulateRequest(PerformancePredictRequest):
    """A performance prediction's inputs plus the draw count and seed. The simulator (L2-C)
    runs only for formats with an innings length (T20, T20I, ODI)."""

    n_samples: int = Field(default=2000, ge=100, le=20000, description="Draws; the served default is 2000")
    seed: int = Field(default=0, ge=0, description="Draws are deterministic given the inputs and the seed")
    return_total_draws: bool = Field(
        default=False,
        description=(
            "Return each side's total for every draw, not only its quantiles (P3-2). A caller that pools "
            "one eleven's totals across several grounds needs the draws: a mixture's quantiles are not the "
            "mean of its parts' quantiles, so a mixed range computed from three summaries would be arithmetic "
            "on the wrong object. Off by default -- it is n_samples floats a side and no surface needs them"
        ),
    )


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
    total_draws: Optional[List[float]] = Field(
        default=None,
        description=(
            "This side's total in every draw, in draw order, present only when the request asked for it "
            "(``return_total_draws``). The same draws ``total`` summarises -- so a caller pooling several "
            "simulations quantifies the pool rather than averaging summaries (P3-2)"
        ),
    )


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
    shared_factor: bool = Field(
        ...,
        description=(
            "Whether the simulator that drew this match carried a shared match factor. A format whose "
            "calibration fold was too thin to fit one ships the un-widened simulator, whose 10-90 "
            "intervals are a different population's (B-12); the track record keeps the two apart"
        ),
    )
    unknown_player_ids: List[str] = Field(default_factory=list)
    served_ratings: ServedRatings
