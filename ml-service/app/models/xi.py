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

from pydantic import BaseModel, Field, field_validator, model_validator

from ml.xi import contract as C

# What an eleven is, everywhere in this module (SERVE-02).
#
# It is a constant rather than a caller's choice because every model behind these routes
# was fitted on elevens and every number they return aggregates one side into one row: the
# objective's XI features are means and counts over eleven, the display model reads the two
# aggregates, and the simulator's innings ends at ten wickets whatever the side holds. A
# nine-player side is therefore answerable -- arithmetic happens and a plausible number
# comes back -- but it answers a different question from the one the caller asked, and
# nothing on the response would say so. So it is refused at the boundary instead (§8.7).
TEAM_SIZE = 11

# The fixture's competition level as a request may name it: the two words Cricsheet writes
# in ``team_type`` and go-app stores in ``match.competition_level``, spelled here as the
# literals ``C.COMPETITION_LEVELS`` declares (a test pins the two equal) so an unknown word
# is a 422 naming the field rather than a probability read at a level nobody asked about.
CompetitionLevel = Literal["international", "club"]


def _upper(v: Optional[str]) -> Optional[str]:
    return v if v is None or v == "" else v.strip().upper()


def _format_code(v: Optional[str]) -> str:
    """Upper-case ``format`` and refuse it here if it names no served format.

    An unrecognised format used to reach the registry unchanged, where it failed the same
    way a real format with no loaded model does -- ``XiUnavailable``, a 503 -- so a typo
    read as "the service is unavailable, retry" (SERVE-13) rather than as the caller's own
    mistake. Checked against ``C.FORMAT_CODES`` rather than against what happens to be
    loaded, so the refusal (422, the field named) is the same whether or not a run is
    currently served.
    """
    code = _upper(v) or ""
    if code not in C.FORMAT_CODES:
        raise ValueError(f"format {v!r} is not a served format code; expected one of {C.FORMAT_CODES}")
    return code


def _refuse_repeats(ids: List[str], field_name: str) -> None:
    """Refuse a side that names one player twice.

    One player fills one place. A repeated id is not a bigger side: the optimiser's
    ``key_index`` keeps the last position an id occupies, so a duplicated pool id can be
    chosen twice for one eleven, and a duplicated eleven id is scored as two players.
    """
    seen: set = set()
    repeated: set = set()
    for player_id in ids:
        if player_id in seen:
            repeated.add(player_id)
        seen.add(player_id)
    if repeated:
        repeated_ids = sorted(repeated)
        raise ValueError(f"{field_name} names the same player more than once: {', '.join(repeated_ids)}")


def _refuse_overlap(first: List[str], second: List[str], first_name: str, second_name: str) -> None:
    """Refuse a player who appears on both sides of one match.

    Nobody plays both elevens. Where he does, the side-level aggregates both read him, the
    optimiser can pick him against himself, and go-app's per-id merges would have one
    side's answer overwrite the other's (GO-04).
    """
    shared = sorted(set(first) & set(second))
    if shared:
        raise ValueError(f"{first_name} and {second_name} name the same player: {', '.join(shared)}")


def _refuse_wrong_size(ids: List[str], field_name: str, team_size: int) -> None:
    """Refuse a side that is not an eleven."""
    if len(ids) != team_size:
        raise ValueError(f"{field_name} holds {len(ids)} players and a side is scored as {team_size}")


class XiConstraints(BaseModel):
    team_size: int = Field(
        default=TEAM_SIZE,
        description="Fixed at 11: every model behind these routes was fitted on elevens, so a "
        "different size is refused rather than answered (SERVE-02)",
    )
    min_bowlers: int = Field(default=5, ge=0, le=TEAM_SIZE)
    require_keeper: bool = True
    must_include: List[str] = Field(default_factory=list)
    must_exclude: List[str] = Field(default_factory=list)

    @field_validator("team_size")
    def _only_eleven(cls, v: int) -> int:
        if v != TEAM_SIZE:
            raise ValueError(f"team_size is {TEAM_SIZE}: no other side size is supported by the served models")
        return v


class XiOptimizeRequest(BaseModel):
    format: str
    pool_player_ids: List[str] = Field(
        ...,
        min_length=TEAM_SIZE,
        description="The candidates the eleven is chosen from: distinct registry ids, at least "
        "team_size of them, and none of them on the opposing side",
    )
    opponent_player_ids: List[str] = Field(
        default_factory=list,
        description="The opposing XI the selection is made against; not read by objective='ratings'. "
        "Where it is given it is a full eleven, distinct, and disjoint from the pool",
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
        return _format_code(v)

    @model_validator(mode="after")
    def _one_player_one_place(self) -> "XiOptimizeRequest":
        """The pool is a set of distinct candidates, and the opponent is somebody else's eleven.

        A pool that names one player twice can have him chosen twice for the same eleven;
        a pool that overlaps the opponent's eleven optimises a side against itself. Both
        are refused here rather than searched over, because the search reports neither.
        """
        _refuse_repeats(self.pool_player_ids, "pool_player_ids")
        if len(self.pool_player_ids) < self.constraints.team_size:
            raise ValueError(
                f"pool_player_ids holds {len(self.pool_player_ids)} candidates and the eleven "
                f"needs {self.constraints.team_size}"
            )
        # An empty opponent list is how objective='ratings' says "nobody is being played
        # against"; where one is given it is an eleven, whichever objective reads it.
        if self.opponent_player_ids:
            _refuse_repeats(self.opponent_player_ids, "opponent_player_ids")
            _refuse_wrong_size(self.opponent_player_ids, "opponent_player_ids", self.constraints.team_size)
            _refuse_overlap(self.pool_player_ids, self.opponent_player_ids, "pool_player_ids", "opponent_player_ids")
        return self


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
        default_factory=dict,
        description="P(win) lost if the player were replaced by a par player in his role: the same expected "
        "balls faced and bowled and the same keeper flag, but scoring and conceding exactly what the format "
        "expects off every ball, at the initial rating. His contribution, with the slot he fills held fixed",
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
        return _format_code(v)


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
    """Two elevens, by registry id.

    Each side is exactly ``TEAM_SIZE`` distinct players and no player is on both. The
    models these ids reach cannot express anything else: the objective and the display
    model read per-side aggregates over eleven, the simulator's innings ends at ten
    wickets however many players the side holds, and a player on both sides is read into
    both aggregates. A short or doubled side is therefore answered rather than rejected by
    the arithmetic -- with a number that looks like every other number -- which is why the
    refusal is here (SERVE-02, §8.7).
    """

    format: str
    team1_player_ids: List[str] = Field(
        ..., description=f"The side batting first, as {TEAM_SIZE} distinct registry ids"
    )
    team2_player_ids: List[str] = Field(
        ..., description=f"The other side, as {TEAM_SIZE} distinct registry ids, none of them team1's"
    )
    team1_id: Optional[int] = Field(
        default=None, description="opposition id of the side batting first (for team-level context)"
    )
    team2_id: Optional[int] = None
    venue_id: Optional[int] = None
    team1_bats_first: Optional[bool] = Field(
        default=None, description="Known after the toss; omit before it to average both batting orders"
    )
    competition_level: Optional[CompetitionLevel] = Field(
        default=None,
        description="The fixture's level -- 'international' between national sides, 'club' otherwise -- which "
        "the display model reads as context. Omit where it is not known: the display is then averaged over both "
        "levels, and the response says so (`competition_level_marginalised`)",
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
        return _format_code(v)

    @model_validator(mode="after")
    def _two_elevens(self) -> "XiWinRequest":
        """One player, one side, eleven of them -- checked on both sides and between them."""
        for ids, field_name in (
            (self.team1_player_ids, "team1_player_ids"),
            (self.team2_player_ids, "team2_player_ids"),
        ):
            _refuse_wrong_size(ids, field_name, TEAM_SIZE)
            _refuse_repeats(ids, field_name)
        _refuse_overlap(self.team1_player_ids, self.team2_player_ids, "team1_player_ids", "team2_player_ids")
        return self


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
        ...,
        ge=0,
        le=1,
        description="XI-only model, the value the optimiser maximises. Always marginalised over the "
        "batting order: the objective reads per-side aggregates over eleven and has no batting-order, "
        "toss or home-advantage feature to read, so a named toss does not move it",
    )
    toss_marginalised: bool = Field(
        ...,
        description="True when no batting order was named and `team1_win_probability` was averaged over "
        "both; false when the named order was read. The two are different quantities and they differ by "
        "0.04 on average in TEST, so a caller has to be able to tell which it holds without re-reading "
        "its own request (§8.7). Who won the toss is on no request and is averaged over in both readings; "
        "`objective_probability` is marginalised either way",
    )
    competition_level_marginalised: bool = Field(
        ...,
        description="True when the request named no `competition_level` and `team1_win_probability` was "
        "averaged over both levels; false when the display model read the level it was given. A fixture's "
        "level is a fact, so an answer that had to average over it is a substitution and says so (§8.7); "
        "`objective_probability` never reads the level",
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
    refused on, so there is one answer rather than two that can disagree.

    The verdict is measured from ``data_through`` -- the run's own training boundary --
    and *not* from ``ratings_through``, the last match it folded in (SERVE-03). The two
    are different quantities and only the first is a fact about the pipeline: the second
    is a fact about the cricket calendar, and a fortnight between Tests is not a fault.
    Measuring the wrong one made the refusal unclearable, because the step its hint names
    cannot move a date that no match moved. Both dates are reported so a reader can see
    the archive's own lag between them without the verdict being taken on it.
    """

    fresh: bool
    data_age_days: Optional[int] = Field(
        default=None,
        description="Days from the run's data boundary (`data_through`) to today -- the quantity "
        "compared against `max_age_days`. Absent when nothing is loaded",
    )
    max_age_days: int
    data_through: Optional[str] = Field(
        default=None,
        description="The run manifest's cutoff: the date the run's data was built to, and what "
        "`data_age_days` counts from",
    )
    ratings_through: Optional[str] = Field(
        default=None,
        description="The last match the run folded in. Reported, never the verdict: in an "
        "off-season it moves for reasons no retrain can change",
    )
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
    and venue ids for context, the toss once known, and ``as_of`` for backtests -- plus
    the two facts about the fixture itself that the feature rows are computed from.

    ``match_date`` is *when the fixture is played*, which is not ``as_of`` (which ratings
    to read) and is certainly not the date of the last match in the state, which is what
    the serving path stamped on every fixture until SERVE-04. Every date-dependent
    feature -- each player's age, and the age-aware cold start's band for a debutant --
    is read at this date, so a fixture next month was being aged as of last month's
    cricket.

    ``gender`` picks the context baseline the fixture's scoring rates come from. It
    matters only where the run was built with the gender split on, and there omitting it
    reads the men's group for a women's fixture (SERVE-01); with the split off there is
    one group and it reads the same either way.
    """

    match_date: Optional[date] = Field(
        default=None,
        description="The day the fixture is played, which every date-dependent feature is read at. "
        "Omitted, the fixture is dated `as_of` if that was given and today otherwise; the response's "
        "`fixture` block says which of the three answered",
    )
    gender: Optional[Literal["male", "female"]] = Field(
        default=None,
        description="Which context baseline the fixture's scoring rates are read from. Only read where "
        "the run was built with the gender split on",
    )


class ServedFixture(BaseModel):
    """The fixture the rows were actually computed for -- the date and the context group,
    as resolved, beside where each came from (SERVE-04, §8.7).

    A caller who sends no ``match_date`` still gets one, because the features cannot be
    computed without a date. What §8.7 forbids is that substitution being invisible: a
    prediction dated by default must say so on the wire, so a reader can tell "the fixture
    I asked about" from "today, because you did not say".
    """

    match_date: str
    match_date_source: Literal["request", "as_of", "today"] = Field(
        description="'request' when the caller sent `match_date`, 'as_of' when the backtest date "
        "dated the fixture, 'today' when neither was given and the service supplied the date"
    )
    gender: Optional[str] = Field(
        default=None,
        description="The context group the fixture read, as the caller named it. Absent where the "
        "caller named none, which reads the unsplit baseline",
    )


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
    recalibrated_targets: List[str] = Field(
        default_factory=list,
        description=(
            "Which of these quantile forecasts the answering model corrects on a temporal fold (H-5). "
            "A target is absent either because its coverage was nominal and no correction was asked for, "
            "or because the model's calibration fold was too thin to fit one -- in both cases the "
            "quantiles here are the model's raw output, and a caller must not read them as corrected "
            "(plan §8.7). The run's report says which of the two it was"
        ),
    )
    served_ratings: ServedRatings
    fixture: ServedFixture = Field(
        ..., description="The fixture these rows were computed for, and where its date came from (SERVE-04)"
    )


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
    """One player's line of the simulation. ``p_bats`` / ``p_bowls`` are the L2-B forecasts
    the draws were made from -- the same numbers ``/performance/predict`` reports for this
    eleven and toss -- and ``batted_share`` / ``bowled_share`` are what the draws realised.
    They differ, and the difference is the simulator's own dynamics: the deliveries budget
    and the chase end an innings before the forecast's depth, and the bowling draft tops
    bowlers up until the side can deliver the innings. Serving the realised share under the
    forecast's name was SERVE-05; §8.7 wants the substitution visible, so both are here."""

    player_id: str
    side: int = Field(..., description="1 = team1, 2 = team2")
    p_bats: float = Field(
        ..., ge=0, le=1, description="L2-B's P(bats) the draws were made from; equals /performance/predict's"
    )
    p_bowls: float = Field(
        ..., ge=0, le=1, description="L2-B's P(bowls) the draws were made from; equals /performance/predict's"
    )
    batted_share: float = Field(
        ...,
        ge=0,
        le=1,
        description="Share of draws in which the player faced a ball: p_bats after the innings' dynamics",
    )
    bowled_share: float = Field(
        ...,
        ge=0,
        le=1,
        description="Share of draws in which the player bowled: p_bowls after the bowling draft's top-up",
    )
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
    competition_level_marginalised: bool = Field(
        ...,
        description="True when the request named no `competition_level` and `win_probability.display` was "
        "averaged over both levels; false when the display model read the level it was given (§8.7). The "
        "draws themselves never read the level",
    )
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
    fixture: ServedFixture = Field(
        ..., description="The fixture these draws were computed for, and where its date came from (SERVE-04)"
    )
