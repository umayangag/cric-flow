"""The chronological rating pass.

``RatingState`` holds every as-of quantity. For each match, in date order, the caller first
reads features from the state (``side_vectors`` / ``aggregate_side`` / ``team_context``) and
only then calls ``update`` with that match. Nothing a feature reads can have seen the match
it describes -- that is the whole leakage guarantee, and it is structural rather than a query
convention.

Impact ratings are relative to a context baseline: the running average runs-per-ball and
wickets-per-ball for (format, over number). A batter's ``bat_rate`` is therefore "runs above
what an average ball in that over yields", so a death-overs 150 strike rate is worth more
than a powerplay one, and the ratings are already venue- and era-neutral in the mean.
"""

from __future__ import annotations

import math
from collections import defaultdict
from copy import copy as shallow_copy
from dataclasses import dataclass, field
from datetime import date
from typing import Dict, List, Mapping, Optional, Sequence, Tuple

import numpy as np

from ml.xi import contract as C
from ml.xi.biography import age_vectors
from ml.xi.geography import NO_COUNTRIES, modal_region, venue_region
from ml.xi.sequence import sequence_flags
from ml.xi.sources import Deliveries, MatchRecord, batting_positions

_N_FMT = len(C.FORMAT_CODES)
_N_SEQ = len(C.PLAYER_SEQUENCE_KEYS)
_SEQ_INDEX: Dict[str, int] = {key: i for i, key in enumerate(C.PLAYER_SEQUENCE_KEYS)}
# (numerator prior, denominator prior) per sequence column: rates shrink toward zero over a
# ball prior; the mean spell length shrinks toward two overs over one spell.
_SEQ_PRIOR: Dict[str, tuple] = {"bowl_spell_overs": (2.0, 1.0)}
_SEQ_DEFAULT_PRIOR = (0.0, C.SEQUENCE_PRIOR_BALLS)


def _grow(arr: np.ndarray, n: int, fill: float) -> np.ndarray:
    if arr.shape[-1] >= n:
        return arr
    new_len = max(n, arr.shape[-1] * 2, 1024)
    out = np.full(arr.shape[:-1] + (new_len,), fill, dtype=arr.dtype)
    out[..., : arr.shape[-1]] = arr
    return out


@dataclass
class PlayerIndex:
    """Stable string key -> integer slot."""

    key_to_slot: Dict[str, int] = field(default_factory=dict)
    keys: List[str] = field(default_factory=list)

    def slot(self, key: str) -> int:
        s = self.key_to_slot.get(key)
        if s is None:
            s = len(self.keys)
            self.key_to_slot[key] = s
            self.keys.append(key)
        return s

    def slots(self, keys: Sequence[str]) -> np.ndarray:
        """Slots for ``keys``, assigning one to any key not seen before. The rating pass owns
        this: reading a player must not invent him (see ``read_slots``)."""
        return np.fromiter((self.slot(k) for k in keys), dtype=np.int64, count=len(keys))

    def read_slots(self, keys: Sequence[str], unrated: int) -> np.ndarray:
        """Slots for ``keys`` without assigning any: a key this index has never seen maps to
        ``unrated``, the reserved column that no update ever writes."""
        return np.fromiter((self.key_to_slot.get(k, unrated) for k in keys), dtype=np.int64, count=len(keys))

    def __len__(self) -> int:
        return len(self.keys)


def elo_expected(rating_a: float, rating_b: float) -> float:
    return 1.0 / (1.0 + 10 ** ((rating_b - rating_a) / 400.0))


_N_PHASES = len(C.PHASE_NAMES)
#: What a fixture-context key the state has never seen reads: no runs, no dismissals, no balls.
_NO_SCORING = (0.0, 0.0, 0.0)
#: What a team-level key the state has never seen reads (B-1). Tuples, not the empty list
#: and pair a ``defaultdict`` would have manufactured, so a read cannot hand a caller a
#: container that is then mutated into a state entry by the back door.
_NO_RESULTS: tuple = ()
_NO_VENUE_BAT_FIRST = (0.0, 0.0)
_NO_VENUE_MATCHES = 0
#: The debut accumulators' last axis (X-1b family 3): the sums a debutant's band pools over
#: every debut match before this one -- the impact numerator, the balls, the wicket
#: numerator and the debut appearances, batted (bowled) or not -- exactly the four
#: quantities a player's own ``side_vectors`` are computed from.
DEBUT_IMPACT, DEBUT_BALLS, DEBUT_WICKETS, DEBUT_MATCHES = 0, 1, 2, 3
#: The vector keys the age-band debut prior replaces for a player with no history in the
#: format. Elo and the role keys keep their neutral values: the prior is about what a
#: debutant of that age does with the ball, not where he bats.
DEBUT_PRIOR_KEYS = ("exp_balls_faced", "bat_rate", "bat_wrate", "exp_balls_bowled", "bowl_rate", "bowl_wrate")

# Everything the state is made of, named once. Three consumers read these tuples -- the
# artifact writer and its refusal (``ml.xi.store``), and ``RatingState.snapshot`` -- and
# an accumulator missing from them is a silent defect in all three at once: an array left
# out of the artifact is D-6's IndexError, and one left out of the snapshot is a serving
# read of a state another thread is still writing. ``test_xi_ratings`` walks a fresh
# state's attributes and fails if any of them is absent here.
#
# The player/context split matters to the artifact check: a player array's last axis is
# the player slot, so its width has to cover every registered key; a context array is
# indexed by (group, format[, over]) and checking its width against the players would
# refuse every healthy run.
PLAYER_ARRAY_NAMES = (
    "bat_rae",
    "bat_balls",
    "bat_wae",
    "bowl_rse",
    "bowl_balls",
    "bowl_wae",
    "career",
    "career_all",
    "kept",
    "pelo",
    "bat_pos_sum",
    "bat_pos_n",
    "xi_n",
    "xi_bat_balls",
    "xi_bowl_balls",
    "bat_ph_rae",
    "bat_ph_balls",
    "bowl_ph_rse",
    "bowl_ph_balls",
    "seq_num",
    "seq_den",
)  # fmt: skip

CONTEXT_ARRAY_NAMES = (
    "ctx_balls",
    "ctx_runs",
    "ctx_wickets",
    "ctx_extras",
    "ctx_bowler_extras",
    "ctx_deliveries",
    "ctx_bowler_wickets",
    "ctx_dismissals",
    "ctx_full_innings_deliveries",
    "ctx_full_innings",
    "debut_bat",
    "debut_bowl",
)  # fmt: skip

STATE_ARRAY_NAMES = PLAYER_ARRAY_NAMES + CONTEXT_ARRAY_NAMES

#: The keyed tables beside the arrays: team and venue state, the fixture-context sums
#: (A-1), the players' dates of birth (X-1b), and the home flag's two inputs (FEAT-05):
#: the region each venue is in, static like the birth dates, and per team the regions it
#: has played in so far, advanced at day close like every other accumulator.
STATE_TABLE_NAMES = (
    "team_elo",
    "team_results",
    "head_to_head",
    "venue_bat_first",
    "team_venue_matches",
    "venue_scoring",
    "competition_scoring",
    "birth_dates",
    "venue_countries",
    "team_countries",
)

#: The behaviour switches a rating state carries: the experiment arms that change what the
#: as-of vectors mean rather than what data they were fed. Every one of them is part of the
#: feature definition, is written into the rating artifact, and has to be threaded wherever
#: a state is rebuilt -- the as-of serving path above all, because a live request and an
#: ``as_of`` request that disagree on a flag answer two different models from one artifact
#: (SERVE-07). Register a new flag here and ``RatingState.flags`` carries it to the
#: artifact, to the snapshot and to the serving pass; ``test_xi_ratings`` fails if a
#: constructor switch is added and not registered.
STATE_FLAG_NAMES: Tuple[str, ...] = ("gender_split_context", "age_aware_cold_start")

#: The tables whose values are mutated in place by ``update`` -- a result appended to a
#: form list, a venue's bat-first pair incremented, a ground's scoring sums advanced, a
#: team's region counts stepped. A snapshot has to copy the value as well as the mapping,
#: or the copy moves with the pass.
_TABLES_WITH_MUTABLE_VALUES = (
    "team_results",
    "head_to_head",
    "venue_bat_first",
    "venue_scoring",
    "competition_scoring",
    "team_countries",
)


class RatingState:
    """All as-of state. Arrays are (format, player_slot); grown on demand.

    ``gender_split_context`` is E7 (H-7): when True the context baselines -- expected runs
    and wickets per (format, over) -- are kept separately for women's and men's matches, so
    a woman's impact is measured against women's cricket rather than a blend. Off by
    default; the flag is part of the feature definition and is recorded in the artifact.

    ``birth_dates`` is X-1b's input: a date of birth per player key, read from the source
    once, so ``age_vectors`` can say how old a player is at any match date. It is a static
    fact, not an accumulator -- nothing in ``update`` touches it. ``age_aware_cold_start``
    is X-1b's family 3: when True, ``side_vectors`` reads a player with no history in the
    format and a known age as the as-of debut profile of his age band instead of the
    neutral vector. Off by default; both are recorded in the artifact.

    ``venue_countries`` is the home flag's static input (FEAT-05): the region each venue
    key is in, read from the source once like the birth dates. Its other input, where each
    team has played so far, is ``team_countries`` and is an accumulator.
    """

    def __init__(
        self,
        gender_split_context: bool = False,
        birth_dates: Optional[Mapping[str, date]] = None,
        age_aware_cold_start: bool = False,
        venue_countries: Optional[Mapping[str, str]] = None,
    ) -> None:
        self.players = PlayerIndex()
        self.gender_split_context = gender_split_context
        self.birth_dates: Dict[str, date] = dict(birth_dates or {})
        self.age_aware_cold_start = age_aware_cold_start
        self.venue_countries: Dict[str, str] = dict(venue_countries or {})
        n = 1024
        z = lambda: np.zeros((_N_FMT, n))  # noqa: E731
        self.bat_rae, self.bat_balls, self.bat_wae = z(), z(), z()
        self.bowl_rse, self.bowl_balls, self.bowl_wae = z(), z(), z()
        self.career = z()
        self.career_all = np.zeros(n)
        # the keeper ledger (FEAT-10): per player, 1.0 for each match he was seen keeping in,
        # decayed on every later match in which his side's keeper was seen -- one clock,
        # every format, advanced only by a match that named the side's keeper
        self.kept = np.zeros(n)
        self.pelo = np.full((_N_FMT, n), C.ELO_INITIAL)
        # expected batting slot: decayed sum of positions batted, decayed count of innings
        # batted, decayed count of XI appearances
        self.bat_pos_sum, self.bat_pos_n, self.xi_n = z(), z(), z()
        # expected involvement: balls faced and bowled, decayed on the XI clock like
        # ``xi_n`` -- on every appearance, not only the matches with a ball -- so that
        # ``exp_balls_faced`` is balls per XI appearance. ``bat_balls`` / ``bowl_balls``
        # above are the impact rates' denominators and keep the rates' own clock (FEAT-01).
        self.xi_bat_balls, self.xi_bowl_balls = z(), z()
        # per-phase impact: (format, phase, player) decayed sums and ball counts
        zp = lambda: np.zeros((_N_FMT, _N_PHASES, n))  # noqa: E731
        self.bat_ph_rae, self.bat_ph_balls = zp(), zp()
        self.bowl_ph_rse, self.bowl_ph_balls = zp(), zp()
        # sequence families (E1): (column, format, player) decayed numerators and denominators
        self.seq_num, self.seq_den = np.zeros((_N_SEQ, _N_FMT, n)), np.zeros((_N_SEQ, _N_FMT, n))
        # context baselines: (gender group, format, over) cumulative balls / runs / wickets
        # with a weak prior. Group 0 is everyone; group 1 is women's matches when
        # gender_split_context is on, otherwise unused.
        self.ctx_balls = np.ones((2, _N_FMT, C.MAX_OVER_INDEX))
        self.ctx_runs = np.full((2, _N_FMT, C.MAX_OVER_INDEX), 1.2)
        self.ctx_wickets = np.full((2, _N_FMT, C.MAX_OVER_INDEX), 0.05)
        # simulator context (L2-C): per (context group, format) running sums over deliveries
        # -- extras, the bowler-charged extras (wides and no-balls, FEAT-08), deliveries,
        # bowler-credited and total dismissals -- and over full first innings (not all out,
        # so they ran their overs) their deliveries and count. The priors are the laws of the
        # game, not tuned values: one innings of legal balls, no extras over one delivery,
        # every dismissal the bowler's over one dismissal.
        self.ctx_extras = np.zeros((2, _N_FMT))
        self.ctx_bowler_extras = np.zeros((2, _N_FMT))
        self.ctx_deliveries = np.ones((2, _N_FMT))
        self.ctx_bowler_wickets = np.ones((2, _N_FMT))
        self.ctx_dismissals = np.ones((2, _N_FMT))
        self.ctx_full_innings_deliveries = np.tile(
            np.asarray([float(C.INNINGS_LEGAL_BALLS[f] or 0) for f in C.FORMAT_CODES]), (2, 1)
        )
        self.ctx_full_innings = np.ones((2, _N_FMT))
        # team-level state
        self.team_elo: Dict[tuple, float] = defaultdict(lambda: C.ELO_INITIAL)
        self.team_results: Dict[tuple, List[float]] = defaultdict(list)
        self.head_to_head: Dict[tuple, List[float]] = defaultdict(list)
        self.venue_bat_first: Dict[tuple, List[float]] = defaultdict(lambda: [0.0, 0.0])
        self.team_venue_matches: Dict[tuple, int] = defaultdict(int)
        # fixture context (A-1): per (format, venue) and (format, competition) the as-of
        # [runs, dismissals, deliveries] over every ball of every match under that key
        self.venue_scoring: Dict[tuple, List[float]] = defaultdict(lambda: [0.0, 0.0, 0.0])
        self.competition_scoring: Dict[tuple, List[float]] = defaultdict(lambda: [0.0, 0.0, 0.0])
        # home advantage (FEAT-05): per team key, how many past matches it played in each
        # region -- every format together, since where a club is from does not depend on
        # the format it is playing. Plain inner dicts: a read must not create a key.
        self.team_countries: Dict[str, Dict[str, int]] = defaultdict(dict)
        # age-band debut profiles (X-1b family 3): per (format, age band) the lifetime sums
        # of what debutants of that band did in their debut match, batting and bowling
        # (``DEBUT_IMPACT`` .. ``DEBUT_MATCHES``). Not decayed: it is a population prior.
        self.debut_bat = np.zeros((_N_FMT, C.N_AGE_BANDS, 4))
        self.debut_bowl = np.zeros((_N_FMT, C.N_AGE_BANDS, 4))
        self.matches_seen = 0
        self.last_date = None

    # -- capacity ------------------------------------------------------------------------
    def _ensure(self, n: int) -> None:
        if self.bat_rae.shape[1] >= n:
            return
        for name in (
            "bat_rae", "bat_balls", "bat_wae", "bowl_rse", "bowl_balls", "bowl_wae", "career",
            "bat_pos_sum", "bat_pos_n", "xi_n", "xi_bat_balls", "xi_bowl_balls",
            "bat_ph_rae", "bat_ph_balls", "bowl_ph_rse", "bowl_ph_balls", "seq_num", "seq_den",
        ):  # fmt: skip
            setattr(self, name, _grow(getattr(self, name), n, 0.0))
        self.career_all = _grow(self.career_all, n, 0.0)
        self.kept = _grow(self.kept, n, 0.0)
        self.pelo = _grow(self.pelo, n, C.ELO_INITIAL)

    def _slots(self, keys: Sequence[str]) -> np.ndarray:
        """Slots for the rating pass, which may bring a player into the state."""
        s = self.players.slots(keys)
        self._ensure(len(self.players))
        return s

    def _read_slots(self, keys: Sequence[str]) -> np.ndarray:
        """Slots for a read, which may not.

        A serving request naming someone the state has never seen -- a debutant, or a caller
        with the wrong kind of id -- used to *append* him, so the loaded state drifted with
        traffic and ``known_players`` stopped reporting him unknown on the second identical
        request. Unknown keys map instead to one reserved column past the last player, which
        `_ensure` initialises exactly as a fresh slot and no update ever writes, so the
        numbers a debutant gets are unchanged and the state is not.
        """
        unrated = len(self.players)
        self._ensure(unrated + 1)
        return self.players.read_slots(keys, unrated)

    def reserve_read_capacity(self) -> None:
        """Make room for the reserved unrated column now, so no read has to.

        ``_read_slots`` grows the arrays when the reserved column is past their width,
        which is a *write* on the read path: harmless while one thread reads, a data race
        once several do (SERVE-01). A state that will be read concurrently -- one loaded
        from an artifact, or a serving snapshot -- reserves the column once while it is
        still private to one thread, after which every read is a pure read.
        """
        self._ensure(len(self.players) + 1)

    def flags(self) -> Dict[str, bool]:
        """Every behaviour switch this state was built with, by name (``STATE_FLAG_NAMES``).

        The one way to ask a state what arms it is running, so the artifact writer, the
        snapshot and the as-of serving pass all rebuild a state from one description
        instead of each naming the flags it happens to know about (SERVE-07).
        """
        return {name: bool(getattr(self, name)) for name in STATE_FLAG_NAMES}

    def snapshot(self) -> "RatingState":
        """An independent copy of this state: what a reader is handed while the pass that
        produced it goes on advancing (SERVE-01).

        The as-of pass mutates one state in place -- arrays, Elo tables, the form lists --
        so handing a request the live object means its ratings change under it while it is
        being read, and two requests a day apart can read the same numbers. Every array is
        copied, and so is every table value the pass mutates rather than replaces; the
        player index is copied too, so a later match cannot register a player into a state
        a reader believes is finished.

        The copy is a full ``RatingState`` and not a frozen shell: ``update`` would work on
        it. What makes it safe to share is that nothing else holds a reference to it. The
        price is the state's own size -- 24 MB of arrays and ~3.5 ms for a 13.5k-player
        state, against an as-of sweep that folds thousands of matches -- which is why
        ``XiRegistry`` keeps one snapshot per as-of date rather than one per request.
        """
        copy = RatingState(**self.flags())
        copy.players = PlayerIndex(key_to_slot=dict(self.players.key_to_slot), keys=list(self.players.keys))
        for name in STATE_ARRAY_NAMES:
            setattr(copy, name, np.copy(getattr(self, name)))
        for name in STATE_TABLE_NAMES:
            source = getattr(self, name)
            table = getattr(copy, name)
            if name in _TABLES_WITH_MUTABLE_VALUES:
                table.update({key: shallow_copy(value) for key, value in source.items()})
            else:
                table.update(source)
        copy.matches_seen = self.matches_seen
        copy.last_date = self.last_date
        copy.reserve_read_capacity()
        return copy

    # -- reads ---------------------------------------------------------------------------
    def age_vectors(self, player_keys: Sequence[str], on: date) -> Dict[str, np.ndarray]:
        """``contract.AGE_COLS`` for the players at ``on``: age in years and whether a date
        of birth exists (0.0 / 0.0 when it does not -- a category, never an imputed age)."""
        return age_vectors(self.birth_dates, player_keys, on)

    def side_vectors(
        self, format_code: str, player_keys: Sequence[str], on: Optional[date] = None
    ) -> Dict[str, np.ndarray]:
        """Per-player as-of vectors for one side (``contract.PLAYER_VECTOR_KEYS`` plus
        ``contract.PLAYER_ROLE_KEYS``). One read path for the win features, the optimiser
        and the player-match rows, so training and serving cannot compute different
        functions of the same eleven names.

        ``on`` is the date the eleven is read at, which only the age-aware cold start
        (X-1b family 3) consumes: a player with no history in the format and a known age
        then reads his age band's as-of debut profile instead of the neutral vector. The
        rows pass the match date; a caller without one reads the state's own date.

        Every shrunk rate is hierarchical (FEAT-06): it shrinks toward the player's own
        rate in the *other* formats, not toward zero, so a T20I regular making his IPL
        debut reads as himself rather than as an unknown. ``hierarchical_rate`` says how;
        a player with no history outside the format reads exactly what he always did.
        Involvement (``exp_balls_*``), Elo and the role keys are not pooled: balls per
        appearance are not comparable across a 120-ball and a 300-ball innings, so a
        format debutant's impact still weighs nothing in the side aggregates until he has
        appeared -- the performance model, which reads the rates directly, is where the
        pooled estimate reaches a prediction first.
        """
        f = C.FORMAT_INDEX[format_code]
        s = self._read_slots(player_keys)
        appearances = self.xi_n[f, s]
        out = {
            "exp_balls_faced": per_xi_appearance(self.xi_bat_balls[f, s], appearances),
            "exp_balls_bowled": per_xi_appearance(self.xi_bowl_balls[f, s], appearances),
            "bat_rate": hierarchical_rate(self.bat_rae, self.bat_balls, f, s, 0.0, C.PRIOR_BALLS),
            "bat_wrate": hierarchical_rate(self.bat_wae, self.bat_balls, f, s, 0.0, C.PRIOR_BALLS),
            "bowl_rate": hierarchical_rate(self.bowl_rse, self.bowl_balls, f, s, 0.0, C.PRIOR_BALLS),
            "bowl_wrate": hierarchical_rate(self.bowl_wae, self.bowl_balls, f, s, 0.0, C.PRIOR_BALLS),
            "career": self.career[f, s].copy(),
            "career_all": self.career_all[s].copy(),
            "pelo": self.pelo[f, s].copy(),
            "keeper": self.kept[s].copy(),
            "exp_bat_position": (self.bat_pos_sum[f, s] + C.BAT_POSITION_PRIOR * C.BAT_POSITION_PRIOR_INNINGS)
            / (self.bat_pos_n[f, s] + C.BAT_POSITION_PRIOR_INNINGS),
            "bat_innings_share": per_xi_appearance(self.bat_pos_n[f, s], appearances),
        }
        for p, name in enumerate(C.PHASE_NAMES):
            out[f"bat_{name}_rate"] = hierarchical_rate(
                self.bat_ph_rae[:, p, :], self.bat_ph_balls[:, p, :], f, s, 0.0, C.PHASE_PRIOR_BALLS
            )
            out[f"bowl_{name}_rate"] = hierarchical_rate(
                self.bowl_ph_rse[:, p, :], self.bowl_ph_balls[:, p, :], f, s, 0.0, C.PHASE_PRIOR_BALLS
            )
        for key, i in _SEQ_INDEX.items():
            prior_num, prior_den = _SEQ_PRIOR.get(key, _SEQ_DEFAULT_PRIOR)
            out[key] = hierarchical_rate(self.seq_num[i], self.seq_den[i], f, s, prior_num, prior_den)
        if self.age_aware_cold_start:
            self._apply_debut_prior(f, player_keys, out, on if on is not None else self.last_date)
        return out

    def _apply_debut_prior(self, f: int, player_keys: Sequence[str], out: Dict[str, np.ndarray], on) -> None:
        """Overwrite ``DEBUT_PRIOR_KEYS`` for the players with no history in the format and
        a known age at ``on`` with their band's debut profile. Everyone else -- every player
        with a match behind him, and every debutant without a date of birth -- is untouched,
        which is what the gate's third condition asks for."""
        if on is None:
            return
        debut = out["career"] == 0
        if not debut.any():
            return
        ages = self.age_vectors(player_keys, on)
        mask = debut & (ages["age_known"] > 0)
        if not mask.any():
            return
        bands = C.age_band(ages["age"][mask])
        prior = debut_prior_vectors(self.debut_bat[f], self.debut_bowl[f], bands)
        for key in DEBUT_PRIOR_KEYS:
            out[key][mask] = prior[key]

    def _ctx_group(self, gender: str) -> int:
        """Which context-baseline group a match belongs to (E7).

        The literal comes from ``contract`` rather than from here: go-app writes this value
        and the contract publishes it, so a private copy is a seam nothing checks (H-24)."""
        return 1 if self.gender_split_context and gender == C.GENDER_FEMALE else 0

    def team_context(self, match: MatchRecord) -> Dict[str, float]:
        """Team-level features (constant w.r.t. the XI).

        ``get``, not indexing: a read must not write a key into the state (the D-7 class,
        B-1). Indexing these ``defaultdict``s meant a request naming a team, a venue or a
        head-to-head pair the state has never seen -- an unknown opposition id, or a
        fixture that names no venue -- grew all five tables by one entry per read, so a
        long-lived store drifted with the traffic it served and an H-8 comparison after
        serving depended on what had been asked for. The defaults below are exactly the
        entries the ``defaultdict``s would have created, so the numbers are unchanged.
        ``update`` keeps its indexing: writing on write is the point there.
        """
        fmt, t1, t2, v = match.format_code, match.team1, match.team2, match.venue
        e1 = self.team_elo.get((fmt, t1), C.ELO_INITIAL)
        e2 = self.team_elo.get((fmt, t2), C.ELO_INITIAL)
        r1 = self.team_results.get((fmt, t1), _NO_RESULTS)[-C.TEAM_FORM_WINDOW :]
        r2 = self.team_results.get((fmt, t2), _NO_RESULTS)[-C.TEAM_FORM_WINDOW :]
        hh = self.head_to_head.get((fmt, t1, t2), _NO_RESULTS)[-C.HEAD_TO_HEAD_WINDOW :]
        vb = self.venue_bat_first.get((fmt, v), _NO_VENUE_BAT_FIRST)
        home1, home2 = self.home_sides(match)
        return {
            "team_elo_diff": e1 - e2,
            "team_form_diff": (float(np.mean(r1)) if r1 else 0.5) - (float(np.mean(r2)) if r2 else 0.5),
            "team_h2h": float(np.mean(hh)) if hh else 0.5,
            "team_h2h_n": float(len(hh)),
            "venue_bf_rate": (vb[0] + 0.5 * C.VENUE_PRIOR_MATCHES) / (vb[1] + C.VENUE_PRIOR_MATCHES),
            "venue_n": vb[1],
            "venue_fam_diff": math.log1p(self.team_venue_matches.get((t1, v), _NO_VENUE_MATCHES))
            - math.log1p(self.team_venue_matches.get((t2, v), _NO_VENUE_MATCHES)),
            "home_diff": home1 - home2,
        }

    def home_sides(self, match: MatchRecord) -> tuple:
        """Whether each side is at home (1.0 / 0.0), as-of (FEAT-05): the ground's region
        (``venue_countries``, static) against the region each side has played in most
        before today (``team_countries``, advanced at day close). A ground the table does
        not place, or a side whose past says nothing yet, reads 0.0 -- the same reading a
        neutral ground gets. ``get``, not indexing: a read must not write a key (B-1)."""
        region = venue_region(self.venue_countries, match.venue)
        if not region:
            return 0.0, 0.0
        return (
            float(modal_region(self.team_countries.get(match.team1, NO_COUNTRIES)) == region),
            float(modal_region(self.team_countries.get(match.team2, NO_COUNTRIES)) == region),
        )

    def fixture_context(self, match: MatchRecord) -> Dict[str, float]:
        """The scoring level of the ground and of the competition, as-of
        (``contract.FIXTURE_CONTEXT_COLS``): the key's runs (dismissals) per delivery over
        the format's, shrunk toward the format's over ``FIXTURE_CONTEXT_PRIOR_BALLS``.

        Written as 1 + (R_k - B_k * r) / ((B_k + P) * r) -- the key's runs above the
        format's expectation, shrunk, over the expectation -- which is the shape the impact
        ratings have and reads exactly 1.0 at zero deliveries, so a key the state has never
        seen and a fixture that names none (a serving request without a venue or a
        competition) get the neutral value from the formula, not from a second code path.
        The reference is the unsplit format baseline (context group 0), as the accumulators
        are: a women's match at a men's ground reads the ground's blended rate.
        """
        f = C.FORMAT_INDEX[match.format_code]
        balls = float(self.ctx_balls[0, f].sum())
        format_runs_per_ball = float(self.ctx_runs[0, f].sum()) / balls
        format_wickets_per_ball = float(self.ctx_wickets[0, f].sum()) / balls
        out: Dict[str, float] = {}
        # ``get``, not indexing: a read must not write a key into the state (the D-7 class).
        for family, sums in (
            ("venue", self.venue_scoring.get((match.format_code, match.venue), _NO_SCORING)),
            ("competition", self.competition_scoring.get((match.format_code, match.competition), _NO_SCORING)),
        ):
            run_col, wicket_col = C.FIXTURE_CONTEXT_FAMILIES[family]
            out[run_col] = _shrunk_relative_rate(sums[0], sums[2], format_runs_per_ball)
            out[wicket_col] = _shrunk_relative_rate(sums[1], sums[2], format_wickets_per_ball)
        return out

    def simulation_context(self, format_code: str, gender: str) -> Dict[str, float]:
        """The as-of rates the simulator consumes (``contract.SIMULATION_CONTEXT_COLS``):
        extras per delivery, the bowler-charged extras (wides and no-balls) per delivery,
        deliveries per full first innings and the bowler-credited share of dismissals, for
        the format (and the match's context group, which is everyone unless the gender
        split is on -- a serving request names no gender and reads group 0, exactly what
        training reads with the split off)."""
        g, f = self._ctx_group(gender), C.FORMAT_INDEX[format_code]
        return {
            "ctx_extras_per_ball": float(self.ctx_extras[g, f] / self.ctx_deliveries[g, f]),
            "ctx_bowler_extras_per_ball": float(self.ctx_bowler_extras[g, f] / self.ctx_deliveries[g, f]),
            "ctx_innings_deliveries": float(self.ctx_full_innings_deliveries[g, f] / self.ctx_full_innings[g, f]),
            "ctx_bowler_wicket_share": float(self.ctx_bowler_wickets[g, f] / self.ctx_dismissals[g, f]),
        }

    # -- update --------------------------------------------------------------------------
    def update(self, match: MatchRecord) -> None:
        """Fold one match into the state. Must be called after its features were read."""
        f = C.FORMAT_INDEX[match.format_code]
        s1, s2 = self._slots(match.team1_players), self._slots(match.team2_players)
        d = match.deliveries
        both = np.concatenate([s1, s2])
        if len(d):
            debut_bands = self._debut_bands(f, both, list(match.team1_players) + list(match.team2_players), match)
            self._update_impact(f, self._ctx_group(match.gender), match.format_code, d, debut_bands)
            self._update_keepers(match, d)
            self._update_simulation_context(f, self._ctx_group(match.gender), d)
            self._update_fixture_context(match, d)
        self.career[f, both] += 1.0
        self.career_all[both] += 1.0
        # Expected batting slot and involvement: every XI member's accumulators decay
        # together, so the mean position is unchanged by matches not batted in while the
        # batted share and the balls per appearance are.
        for name in ("bat_pos_sum", "bat_pos_n", "xi_n", "xi_bat_balls", "xi_bowl_balls"):
            getattr(self, name)[f, both] *= C.DECAY_PER_MATCH
        self.xi_n[f, both] += 1.0
        if len(d):
            in_xi = set(both.tolist())
            for key, position in batting_positions(d).items():
                slot = self.players.key_to_slot.get(key)
                if slot in in_xi:
                    self.bat_pos_sum[f, slot] += float(position)
                    self.bat_pos_n[f, slot] += 1.0
            self._accumulate_involvement(f, both, d)
        self.team_venue_matches[(match.team1, match.venue)] += 1
        self.team_venue_matches[(match.team2, match.venue)] += 1
        region = venue_region(self.venue_countries, match.venue)
        if region:
            for team in (match.team1, match.team2):
                played = self.team_countries[team]
                played[region] = played.get(region, 0) + 1
        y = match.outcome
        fmt = match.format_code
        if y is not None:
            k1, k2 = (fmt, match.team1), (fmt, match.team2)
            e1, e2 = self.team_elo[k1], self.team_elo[k2]
            exp1 = elo_expected(e1, e2)
            self.team_elo[k1] = e1 + C.K_TEAM_ELO * (y - exp1)
            self.team_elo[k2] = e2 + C.K_TEAM_ELO * ((1.0 - y) - (1.0 - exp1))
            self.team_results[k1].append(y)
            self.team_results[k2].append(1.0 - y)
            self.head_to_head[(fmt, match.team1, match.team2)].append(y)
            self.head_to_head[(fmt, match.team2, match.team1)].append(1.0 - y)
            vb = self.venue_bat_first[(fmt, match.venue)]
            vb[0] += y
            vb[1] += 1.0
            p_exp = elo_expected(float(self.pelo[f, s1].mean()), float(self.pelo[f, s2].mean()))
            self.pelo[f, s1] += C.K_PLAYER_ELO * (y - p_exp)
            self.pelo[f, s2] += C.K_PLAYER_ELO * ((1.0 - y) - (1.0 - p_exp))
        elif match.drawn_or_tied:
            self.team_results[(fmt, match.team1)].append(0.5)
            self.team_results[(fmt, match.team2)].append(0.5)
        self.matches_seen += 1
        # The day the state's cricket runs through: the latest last day folded (FEAT-09).
        self.last_date = match.last_day if self.last_date is None else max(self.last_date, match.last_day)

    def fold_finished(self, pending: Sequence[MatchRecord], before: date) -> List[MatchRecord]:
        """Fold, in order, every pending match whose last day is before ``before``, and
        return the ones still being played (FEAT-09).

        The one rule for the training pass and the as-of serving path: a match joins the
        state once its last day has closed, so a fixture dated during a Test reads a state
        without that Test -- as nobody on that day could have known its later days -- and
        the two paths agree on it (H-8). A match on its one day is folded at that day's
        close, as it always was.
        """
        still_playing: List[MatchRecord] = []
        for match in pending:
            if match.last_day < before:
                self.update(match)
            else:
                still_playing.append(match)
        return still_playing

    def _accumulate_involvement(self, f: int, xi_slots: np.ndarray, d: Deliveries) -> None:
        """Land this match's deliveries on the XI members' involvement numerators -- every
        delivery, wides included, as every ball count in the pass is. Only the players
        named for the match take part: a ball by anyone else has no appearance to be
        divided by, and landing it would put his numerator on no clock at all."""
        for balls, who in ((self.xi_bat_balls, d.batter), (self.xi_bowl_balls, d.bowler)):
            slots = self._slots(list(who))
            named = np.isin(slots, xi_slots)
            np.add.at(balls[f], slots[named], 1.0)

    def _debut_bands(self, f: int, slots: np.ndarray, keys: Sequence[str], match: MatchRecord) -> Dict[int, int]:
        """Slot -> age band for the XI members making their debut in the format today
        whose age is known: the players whose debut match joins their band's profile once
        the day closes. Read before ``career`` is incremented, so it is the same population
        ``side_vectors`` would have called debutants when the row was built."""
        debut = self.career[f, slots] == 0
        if not debut.any():
            return {}
        ages = self.age_vectors(keys, match.match_date)
        mask = debut & (ages["age_known"] > 0)
        bands = C.age_band(ages["age"])
        return {int(slot): int(band) for slot, band in zip(slots[mask], bands[mask])}

    def _update_impact(
        self, f: int, g: int, format_code: str, d: Deliveries, debut_bands: Optional[Dict[int, int]] = None
    ) -> None:
        over = np.minimum(d.over, C.MAX_OVER_INDEX - 1)
        exp_runs = self.ctx_runs[g, f, over] / self.ctx_balls[g, f, over]
        exp_wk = self.ctx_wickets[g, f, over] / self.ctx_balls[g, f, over]
        batters = self._slots(list(d.batter))
        bowlers = self._slots(list(d.bowler))
        ones = np.ones(len(d))
        # The batter's ledger charges each dismissal to the batter who was out (FEAT-07):
        # the striker his own, against the over's expectation on the ball he faced, and a
        # batter out at the other end -- a run out backing up, an incomer timed out -- his,
        # on no ball of his own. ``d.wicket`` counts every wicket the innings lost and stays
        # the baseline's term, so the striker's expectation is the innings' wicket rate
        # while his indicator is his own dismissal: what the ``dismissals`` target counts.
        own_dismissals, dismissed_elsewhere = batter_dismissals(d, self._slots)
        n_elsewhere = len(dismissed_elsewhere)
        bat_who = np.concatenate([batters, dismissed_elsewhere])
        bat_runs = np.concatenate([d.runs_batter - exp_runs, np.zeros(n_elsewhere)])
        bat_wickets = np.concatenate([exp_wk - own_dismissals, -np.ones(n_elsewhere)])
        bat_balls = np.concatenate([ones, np.zeros(n_elsewhere)])
        if debut_bands:
            self._accumulate_debut(self.debut_bat[f], debut_bands, bat_who, bat_runs, bat_wickets, bat_balls)
            self._accumulate_debut(
                self.debut_bowl[f], debut_bands, bowlers, exp_runs - d.runs_bowler, d.bowler_wicket - exp_wk, ones
            )
        self._accumulate(self.bat_rae, self.bat_balls, self.bat_wae, f, bat_who, bat_runs, bat_wickets, bat_balls)
        # The bowler's ledger charges him ``runs_bowler`` -- the total less byes, leg-byes
        # and penalty runs -- against the over's expectation, which stays the innings'
        # total. Charging him the total put his keeper's misses in ``bowl_rate`` (FEAT-08).
        self._accumulate(
            self.bowl_rse,
            self.bowl_balls,
            self.bowl_wae,
            f,
            bowlers,
            exp_runs - d.runs_bowler,
            d.bowler_wicket - exp_wk,
            ones,
        )
        mid_start, death_start = C.PHASE_BOUNDS[format_code]
        phase = np.where(d.over >= death_start, 2, np.where(d.over >= mid_start, 1, 0))
        self._accumulate_phase(self.bat_ph_rae, self.bat_ph_balls, f, batters, phase, d.runs_batter - exp_runs)
        self._accumulate_phase(self.bowl_ph_rse, self.bowl_ph_balls, f, bowlers, phase, exp_runs - d.runs_bowler)
        self._update_sequence(f, d, batters, bowlers, exp_runs, exp_wk)
        np.add.at(self.ctx_balls[g, f], over, 1.0)
        np.add.at(self.ctx_runs[g, f], over, d.runs_total)
        np.add.at(self.ctx_wickets[g, f], over, d.wicket)

    def _update_keepers(self, match: MatchRecord, d: Deliveries) -> None:
        """Advance the keeper ledger for each side whose keeper this match named (FEAT-10).

        A stumping names the keeper; nothing else in the archive does -- Cricsheet marks no
        fielder as the keeper on a catch, so the "catches as keeper" the finding asked for
        cannot be read. A match therefore says who kept for a side only when that side was
        credited a stumping, and then it says it about all eleven: the stumper kept, the
        other ten did not. Such a side's members are decayed once and the stumper lands a
        full match on his weight; a side that recorded none says nothing and moves nobody.
        Until FEAT-10 one stumping set a global, permanent flag: 92 of the archive's 1,060
        stumpers had it through 31 or more later appearances without another, a run no
        keeper has at the format rates of 0.30-0.40 stumping matches per match. The clock
        is the side's identified matches and not the player's appearances, so a keeper is
        not forgotten for a run of matches in which nobody was stumped; its rate and the
        bar on the weight are set beside it in the contract.

        One ledger for every format, not one per format: who keeps is a fact about the
        player, and a per-format ledger would read 0 for a real keeper in the format his
        side has yet to record a stumping in -- which is the defect's third clause.
        """
        sides = (match.team1_players, match.team2_players)
        stumpers: List[set] = [set(), set()]
        for i in np.flatnonzero(d.stumping):
            for key in d.fielders[i] if i < len(d.fielders) else []:
                for side_index, members in enumerate(sides):
                    if key in members:
                        stumpers[side_index].add(key)
        for side_index, members in enumerate(sides):
            if not stumpers[side_index]:
                continue
            self.kept[self._slots(list(members))] *= C.KEEPER_DECAY_PER_IDENTIFIED_MATCH
            self.kept[self._slots(sorted(stumpers[side_index]))] += 1.0

    def _update_simulation_context(self, f: int, g: int, d: Deliveries) -> None:
        self.ctx_extras[g, f] += float((d.runs_total - d.runs_batter).sum())
        # The bowler's charge less the batter's runs is the wides and no-balls: the extras
        # the simulator may put on a bowler's figures. Byes, leg-byes and penalties are the
        # innings' (FEAT-08), and the attribution must not sum them into ``runs_conceded``.
        self.ctx_bowler_extras[g, f] += float((d.runs_bowler - d.runs_batter).sum())
        self.ctx_deliveries[g, f] += float(len(d))
        self.ctx_bowler_wickets[g, f] += float(d.bowler_wicket.sum())
        self.ctx_dismissals[g, f] += float(d.wicket.sum())
        first = d.innings == 0
        # A first innings that was not all out ran its overs (rain aside), so its length in
        # deliveries -- wides included, as every ball count here is -- is the innings length.
        # Index 0 and not the smallest present: a forfeited first innings bowled nothing, and
        # calling whichever innings did bowl "the first" would measure a chase as an innings
        # that ran its overs (IMPORT-12). A match with no first innings contributes none.
        if first.any() and d.wicket[first].sum() < C.MAX_WICKETS:
            self.ctx_full_innings_deliveries[g, f] += float(first.sum())
            self.ctx_full_innings[g, f] += 1.0

    def _update_fixture_context(self, match: MatchRecord, d: Deliveries) -> None:
        """Both innings' deliveries land on the ground's and the competition's sums. An
        unnamed key is no key: nothing accumulates under it, so it keeps reading 1.0."""
        runs, wickets, balls = float(d.runs_total.sum()), float(d.wicket.sum()), float(len(d))
        for sums, key in (
            (self.venue_scoring, match.venue),
            (self.competition_scoring, match.competition),
        ):
            if not key:
                continue
            entry = sums[(match.format_code, key)]
            entry[0] += runs
            entry[1] += wickets
            entry[2] += balls

    @staticmethod
    def _accumulate_debut(table: np.ndarray, debut_bands: Dict[int, int], who, value, wvalue, count) -> None:
        """Land a debutant's balls on his age band's lifetime sums (``DEBUT_IMPACT`` ..
        ``DEBUT_MATCHES``): the same per-ball quantities ``_accumulate`` lands on the
        player, pooled by band and never decayed. The appearance counts whether or not he
        got a ball, as ``xi_n`` does for the player himself, so the band's balls per
        appearance is the quantity ``side_vectors`` reads for everyone else."""
        for slot, band in debut_bands.items():
            table[band, DEBUT_MATCHES] += 1.0
            mine = who == slot
            if not mine.any():
                continue
            table[band, DEBUT_IMPACT] += float(value[mine].sum())
            table[band, DEBUT_BALLS] += float(count[mine].sum())
            table[band, DEBUT_WICKETS] += float(wvalue[mine].sum())

    def _accumulate(self, total, balls, wtotal, f, who, value, wvalue, count) -> None:
        uniq, inv = np.unique(who, return_inverse=True)
        for arr in (total, balls, wtotal):
            arr[f, uniq] *= C.DECAY_PER_MATCH
        total[f, uniq] += np.bincount(inv, weights=value, minlength=len(uniq))
        balls[f, uniq] += np.bincount(inv, weights=count, minlength=len(uniq))
        wtotal[f, uniq] += np.bincount(inv, weights=wvalue, minlength=len(uniq))

    def _update_sequence(self, f: int, d: Deliveries, batters, bowlers, exp_runs, exp_wk) -> None:
        """Accumulate the sequence families (E1) from this match's per-ball flags."""
        flags = sequence_flags(d)
        bat_stuck = (flags.bat_dots_before >= C.SEQUENCE_DOT_STREAK).astype(float)
        bowl_squeeze = (flags.bowl_dots_before >= C.SEQUENCE_DOT_STREAK).astype(float)
        bat_above = d.runs_batter - exp_runs
        bowl_saved = exp_runs - d.runs_bowler
        bowl_wickets_above = d.bowler_wicket - exp_wk
        ones = np.ones(len(d))
        first_over = flags.spell_first_over.astype(float)
        after_boundary_bat = flags.bat_after_boundary.astype(float)
        after_boundary_bowl = flags.bowl_after_boundary.astype(float)
        after_wicket = flags.bowl_after_wicket.astype(float)
        self._accumulate_sequence(f, "bat_stuck_share", batters, bat_stuck, ones)
        self._accumulate_sequence(f, "bat_release_rate", batters, bat_above * bat_stuck, bat_stuck)
        self._accumulate_sequence(f, "bowl_squeeze_share", bowlers, bowl_squeeze, ones)
        self._accumulate_sequence(f, "bowl_squeeze_wrate", bowlers, bowl_wickets_above * bowl_squeeze, bowl_squeeze)
        self._accumulate_sequence(
            f, "bat_after_boundary_rate", batters, bat_above * after_boundary_bat, after_boundary_bat
        )
        self._accumulate_sequence(
            f, "bowl_after_boundary_rate", bowlers, bowl_saved * after_boundary_bowl, after_boundary_bowl
        )
        self._accumulate_sequence(f, "bowl_after_wicket_rate", bowlers, bowl_saved * after_wicket, after_wicket)
        self._accumulate_sequence(f, "bowl_spell_first_rate", bowlers, bowl_saved * first_over, first_over)
        self._accumulate_sequence(
            f, "bowl_spell_later_rate", bowlers, bowl_saved * (1.0 - first_over), 1.0 - first_over
        )
        if flags.spells_per_bowler:
            keys = list(flags.spells_per_bowler)
            slots = self._slots(keys)
            overs = np.asarray([flags.spells_per_bowler[k][1] for k in keys], dtype=float)
            spells = np.asarray([flags.spells_per_bowler[k][0] for k in keys], dtype=float)
            self._accumulate_sequence(f, "bowl_spell_overs", slots, overs, spells)

    def _accumulate_sequence(self, f: int, key: str, who, value, count) -> None:
        """One decay per player per match, then the family's masked sums land on the player."""
        i = _SEQ_INDEX[key]
        uniq, inv = np.unique(who, return_inverse=True)
        self.seq_num[i, f, uniq] *= C.DECAY_PER_MATCH
        self.seq_den[i, f, uniq] *= C.DECAY_PER_MATCH
        self.seq_num[i, f, uniq] += np.bincount(inv, weights=value, minlength=len(uniq))
        self.seq_den[i, f, uniq] += np.bincount(inv, weights=count, minlength=len(uniq))

    def _accumulate_phase(self, total, balls, f, who, phase, value) -> None:
        """Like ``_accumulate`` but per innings phase: one decay per player per match,
        additions land in the phase each ball was bowled in."""
        uniq = np.unique(who)
        total[f][:, uniq] *= C.DECAY_PER_MATCH
        balls[f][:, uniq] *= C.DECAY_PER_MATCH
        np.add.at(total[f], (phase, who), value)
        np.add.at(balls[f], (phase, who), 1.0)


def batter_dismissals(d: Deliveries, slots_of) -> tuple:
    """Who each ball's dismissals belong to (FEAT-07): per delivery, how many of them were
    the striker's own, and the slots of everyone dismissed at the other end -- one entry
    per such dismissal, in playing order.

    ``players_out`` names every dismissed player on every ball (IMPORT-06); a delivery with
    no entry dismissed nobody. Until FEAT-07 the ledger read ``wicket`` instead, which
    counts the wickets the innings lost on the ball, and charged them all to the striker:
    11,114 of the archive's 353,063 dismissals -- 11,037 run outs at the non-striker's end,
    the rest retired out, obstructing the field, handled the ball and timed out -- were
    charged to the batter who was facing, and the batter who was out was charged nothing.
    """
    own = np.zeros(len(d))
    elsewhere: List[str] = []
    for i, dismissed in enumerate(d.players_out):
        for key in dismissed:
            if key == d.batter[i]:
                own[i] += 1.0
            else:
                elsewhere.append(key)
    return own, np.asarray(slots_of(elsewhere), dtype=int)


def hierarchical_rate(
    numerator: np.ndarray, denominator: np.ndarray, f: int, s: np.ndarray, prior_num: float, prior_den: float
) -> np.ndarray:
    """A shrunk rate whose prior mean is the player's own rate elsewhere (FEAT-06).

    ``numerator`` and ``denominator`` are (format, player) sums. The plain rate was
    ``(num_f + prior_num) / (den_f + prior_den)``: ``prior_den`` balls' worth of a prior
    whose mean is ``prior_num / prior_den`` -- zero for every impact rate. Here the prior
    keeps its weight and its mean becomes the player's rate over his *other* formats,
    itself shrunk toward the old prior over the same weight::

        prior_mean = (sum_{g != f} num_g + prior_num) / (sum_{g != f} den_g + prior_den)
        rate_f     = (num_f + prior_den * prior_mean) / (den_f + prior_den)

    So a player with no history outside the format reads exactly the plain rate (the
    other-format sums are zero and the prior mean is the old one), a format debutant with
    a career elsewhere reads his rate elsewhere, and a player with balls in both reads a
    blend that his own format's balls take over at the old pace. The format's own sums
    are left out of the prior on purpose: pooling them in would count a single-format
    player's balls twice and shrink everyone less than before.
    """
    own_num, own_den = numerator[f, s], denominator[f, s]
    other_num = numerator[:, s].sum(axis=0) - own_num
    other_den = denominator[:, s].sum(axis=0) - own_den
    prior_mean = (other_num + prior_num) / (other_den + prior_den)
    return (own_num + prior_den * prior_mean) / (own_den + prior_den)


def per_xi_appearance(quantity: np.ndarray, appearances: np.ndarray) -> np.ndarray:
    """A per-appearance mean: ``quantity`` over the appearances it accrued across, both on
    one decay clock (or both undecayed, as the debut tables are). Never divides by less
    than one appearance, which is what a count with at least one appearance always is --
    the latest contributes 1.0 undecayed -- so a player with none reads 0, not a ratio."""
    return quantity / np.maximum(appearances, 1.0)


def debut_prior_vectors(debut_bat: np.ndarray, debut_bowl: np.ndarray, bands: np.ndarray) -> Dict[str, np.ndarray]:
    """``DEBUT_PRIOR_KEYS`` for debutants in ``bands``, from one format's debut tables:
    the band's pooled sums put through the formulas ``side_vectors`` applies to a player's
    own sums -- balls per debut appearance, and each impact shrunk over ``PRIOR_BALLS`` --
    so a band nobody has debuted in yet reads exactly the neutral vector."""
    out: Dict[str, np.ndarray] = {}
    for table, balls_key, rate_key, wrate_key in (
        (debut_bat, "exp_balls_faced", "bat_rate", "bat_wrate"),
        (debut_bowl, "exp_balls_bowled", "bowl_rate", "bowl_wrate"),
    ):
        rows = table[bands]
        balls = rows[:, DEBUT_BALLS]
        out[balls_key] = per_xi_appearance(balls, rows[:, DEBUT_MATCHES])
        out[rate_key] = rows[:, DEBUT_IMPACT] / (balls + C.PRIOR_BALLS)
        out[wrate_key] = rows[:, DEBUT_WICKETS] / (balls + C.PRIOR_BALLS)
    return out


def _shrunk_relative_rate(key_total: float, key_balls: float, format_rate: float) -> float:
    """``fixture_context``'s formula for one column: the key's per-delivery rate over the
    format's, shrunk toward 1.0 over ``FIXTURE_CONTEXT_PRIOR_BALLS`` deliveries."""
    excess = key_total - key_balls * format_rate
    return 1.0 + excess / ((key_balls + C.FIXTURE_CONTEXT_PRIOR_BALLS) * format_rate)


# ---------------------------------------------------------------------------
# Aggregation: per-player vectors -> one side's features. Pure, reused for counterfactual XIs.
# ---------------------------------------------------------------------------


def aggregate_side(vectors: Dict[str, np.ndarray], format_code: str) -> Dict[str, float]:
    ebf, ebb = vectors["exp_balls_faced"], vectors["exp_balls_bowled"]
    bat = vectors["bat_rate"] * ebf
    bat_w = vectors["bat_wrate"] * ebf
    bowl = vectors["bowl_rate"] * ebb
    bowl_w = vectors["bowl_wrate"] * ebb
    bat_sorted = np.sort(bat)[::-1]
    bowl_sorted = np.sort(bowl)[::-1]
    is_bowler = C.is_bowling_option(ebb, format_code)
    career = vectors["career"]
    pelo = vectors["pelo"]
    return {
        "imp_bat_sum": float(bat.sum()),
        "imp_bat_top6": float(bat_sorted[:6].sum()),
        "imp_bat_tail": float(bat_sorted[6:].sum()),
        "imp_bat_wk": float(bat_w.sum()),
        "imp_bowl_sum": float(bowl.sum()),
        "imp_bowl_top5": float(bowl_sorted[:5].sum()),
        "imp_bowl_wk": float(bowl_w.sum()),
        "imp_bowl_wk_top5": float(np.sort(bowl_w)[::-1][:5].sum()),
        "n_bowlers": float(is_bowler.sum()),
        "exp_balls_bowled_top5": float(np.sort(ebb)[::-1][:5].sum()),
        "exp_balls_faced_sum": float(ebf.sum()),
        "has_keeper": float(C.is_keeper(vectors["keeper"]).any()),
        "n_allrounders": float((is_bowler & (ebf >= 0.6 * np.median(ebf + 1e-9))).sum()),
        "exp_mean_matches": float(career.mean()),
        "n_debutants": float((career == 0).sum()),
        "exp_mean_matches_all": float(vectors["career_all"].mean()),
        "pelo_mean": float(pelo.mean()),
        "pelo_top3": float(np.sort(pelo)[::-1][:3].mean()),
        "pelo_min": float(pelo.min()),
        "pelo_std": float(pelo.std()),
    }


def match_features(side1: Dict[str, float], side2: Dict[str, float]) -> Dict[str, float]:
    """Both sides' aggregates -> the XI feature row (``contract.XI_FEATURE_COLS``)."""
    row: Dict[str, float] = {}
    for stem in C.SIDE_FEATURE_STEMS:
        row[f"t1_{stem}"] = side1[stem]
        row[f"t2_{stem}"] = side2[stem]
        row[f"d_{stem}"] = side1[stem] - side2[stem]
    return row


def xi_feature_vector(
    side1: Dict[str, float], side2: Dict[str, float], columns: Optional[List[str]] = None
) -> np.ndarray:
    row = match_features(side1, side2)
    return np.asarray([row[c] for c in (columns or C.XI_FEATURE_COLS)], dtype=float)
