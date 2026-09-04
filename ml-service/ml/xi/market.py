"""X-4: the betting market as a yardstick, never as a feature.

The question this answers is the one a sophisticated buyer asks first: *how far from the
practical ceiling is the display model?* A win probability can only be judged against
another probability for the same match, and the closing betting market is the best publicly
observable forecast there is -- thousands of people with money at stake, priced to the
first ball. So the harness scores the market beside the display model **on the matches both
cover**, per format, and prints the gap.

Three rules hold this apart from every other module here:

* **Never a feature.** Nothing in the feature contract, the rating pass, the win models,
  the performance model or the simulator imports this module. The odds are read at
  evaluation time only, and ``tests/test_xi_market.py`` asserts the import graph stays that
  way. A model that consumed odds would be predicting the market, not the cricket, and
  would be unservable for a fixture the market has not priced.
* **It decides nothing.** The gate registered as ``X-4`` in ``ml.xi.gates`` states it
  informs. No feature ships, no scoping flips and no threshold moves on its result.
* **Coverage is part of the answer.** A benchmark over 12 % of one format's matches is a
  benchmark over 12 % of one format's matches, and the report says so on its face rather
  than printing an AUC that reads like a verdict on the system.

**The source.** Betfair's data-science team publishes per-season CSV summaries of the
Exchange's cricket *Match Odds* markets for the Big Bash League and the Women's Big Bash
League -- one row per runner per market, carrying the best back and lay price at a series of
points, the first of which is the first ball of the match. They are free, need no account,
and are the only cricket odds series found in the X-4 source review that could be used at
all without a paid licence or a gambling account (the review, with the costs of what was
rejected, is in ``docs/EXTERNAL_DATA_PLAN.md``). They are *not* openly licensed: the files
are cached under a documented path and never committed.

**The price used** is ``BEST_BACK_FIRST_BALL``: the best price available to back each side
at the first delivery. It is the closing price, and it is strictly pre-match -- but it is
struck *after* the toss, which the display model's served probability deliberately
marginalises over. That is an information advantage to the market, so the report scores a
third arm beside the other two: the same display model read at the orientation that
actually happened (``display_toss_aware``), which is the like-for-like comparison. Both are
printed, because the served number is the one users see and the toss-aware one is the one
the market's information set matches.

**De-vigging** is proportional: each side's implied probability is ``1 / price``, and the
pair is divided by its sum. On an exchange the excess over 1.0 is the back/lay spread
rather than a bookmaker's margin, and it is small (the mean overround is reported beside
the numbers); the proportional method splits it in proportion to each side's implied
probability, which for a two-runner market is the standard choice.
"""

from __future__ import annotations

import csv
import glob
import logging
import os
import re
from dataclasses import dataclass, field
from datetime import date
from typing import Callable, Dict, Iterable, List, Optional, Sequence, Tuple

import numpy as np
import pandas as pd
from sklearn.metrics import brier_score_loss, roc_auc_score

from ml.xi import contract as C
from ml.xi.train import _xy, marginalised_probabilities

logger = logging.getLogger(__name__)

#: Where the cached odds files live, relative to ml-service, unless configuration or the
#: environment says otherwise. Documented in docs/config-and-data.md; git-ignored, because
#: the source's terms do not permit redistribution.
DEFAULT_CACHE_DIR = os.path.join("..", "data", "market-odds")
CACHE_DIR_ENV = "ML_MARKET_ODDS_DIR"

SOURCE_NAME = "Betfair Exchange Match Odds season summaries (BBL, WBBL)"
SOURCE_URL = "https://betfair-datascientists." + "git" + "hub.io/data/dataListing/"
SOURCE_LICENCE = (
    "published free of charge and without registration; no open licence is granted, so the files are "
    "cached locally and never committed or redistributed"
)
#: Which point in each market's price series is read. The first ball is the close.
PRICED_AT = "BEST_BACK_FIRST_BALL"
PRICED_AT_NOTE = "best back price at the first ball -- the closing pre-match price, struck after the toss"

DEVIG_METHOD = "proportional"
DEVIG_NOTE = (
    "each runner's implied probability is 1 / best back price, and the pair is divided by its sum; the "
    "excess over 1.0 on an exchange is the back/lay spread, not a bookmaker margin"
)

JOIN_KEY_NOTE = (
    "exact match date, plus both sides resolved through the identity layer (club + gender, the D-10 "
    "contract); a quote that resolves to no match, to more than one, or to a team the identity layer "
    "does not know is counted and dropped, never guessed onto a fixture"
)

#: Each cached file's competition and gender, by the prefix of its name. The runner names
#: alone cannot say it -- "Perth Scorchers" fields a men's and a women's side -- and the
#: files' own ``PATH`` column is blank in the older seasons, so the mapping is declared
#: here and a file matching no prefix is counted as unrecognised rather than assumed.
FILE_PREFIXES: Tuple[Tuple[str, str, str], ...] = (
    ("WBBL", "Women's Big Bash League", C.GENDER_FEMALE),
    ("BBL", "Big Bash League", C.GENDER_MALE),
)

#: Competition tokens some seasons append to a runner name ("Perth Scorchers W",
#: "Brisbane Heat WBBL"). Stripped only as a second attempt, after the name as written has
#: failed to resolve, so a real club whose name ends in one of these is never mangled.
RUNNER_NAME_SUFFIX = re.compile(r"\s+(WBBL|BBL|W|M)$", re.IGNORECASE)

#: Replicates behind the pooled delta's interval, and the seed they are drawn under.
BOOTSTRAP_REPLICATES = 2000
BOOTSTRAP_SEED = 0

#: Below this many joined matches a window is reported with its count and no metrics: an
#: AUC over a handful of matches is noise wearing a verdict's clothes.
MIN_SCORED_MATCHES = 20


@dataclass(frozen=True)
class MarketQuote:
    """One two-runner Match Odds market, at its closing price."""

    market_id: str
    event_date: date
    gender: str
    competition: str
    runners: Tuple[str, str]
    back_prices: Tuple[float, float]
    #: The runner the source recorded as the winner, or ``None`` when it recorded neither.
    winner: Optional[str]

    @property
    def overround(self) -> float:
        return float(sum(1.0 / price for price in self.back_prices))

    def implied_probabilities(self) -> Tuple[float, float]:
        """The de-vigged pair, in runner order (``DEVIG_METHOD``)."""
        implied = [1.0 / price for price in self.back_prices]
        total = sum(implied)
        return float(implied[0] / total), float(implied[1] / total)


@dataclass
class LoadCounts:
    """What the cache directory offered and what became a usable quote."""

    files: List[str] = field(default_factory=list)
    unrecognised_files: List[str] = field(default_factory=list)
    rows: int = 0
    quotes: int = 0
    #: Markets dropped before any join: not two runners, or a price that cannot be read.
    unusable: int = 0

    def as_dict(self) -> Dict[str, object]:
        return {
            "files": sorted(self.files),
            "unrecognised_files": sorted(self.unrecognised_files),
            "rows_read": self.rows,
            "quotes_loaded": self.quotes,
            "quotes_unusable": self.unusable,
        }


@dataclass
class JoinCounts:
    """Why every loaded quote did or did not reach a match. The four failure counts and
    the join add up to ``quotes``, so a quiet drop is not a possible outcome."""

    quotes: int = 0
    joined: int = 0
    unknown_team: int = 0
    no_match: int = 0
    ambiguous: int = 0
    #: Joined quotes whose recorded winner is not the side our own result says won. A
    #: non-zero count is a bad join, not a curiosity -- it is the integrity check on the
    #: whole mapping.
    label_disagreements: int = 0

    def as_dict(self) -> Dict[str, object]:
        return {
            "quotes_joined": self.joined,
            "quotes_unknown_team": self.unknown_team,
            "quotes_no_match": self.no_match,
            "quotes_ambiguous": self.ambiguous,
            "label_disagreements": self.label_disagreements,
            "key": JOIN_KEY_NOTE,
        }


def cache_dir(configured: Optional[str] = None) -> str:
    """Where the odds are cached: the explicit argument, then the environment, then the
    documented default."""
    return configured or os.environ.get(CACHE_DIR_ENV) or DEFAULT_CACHE_DIR


def _file_identity(name: str) -> Optional[Tuple[str, str]]:
    for prefix, competition, gender in FILE_PREFIXES:
        if name.upper().startswith(prefix):
            return competition, gender
    return None


def _price(raw: str) -> Optional[float]:
    try:
        value = float(raw)
    except (TypeError, ValueError):
        return None
    # A decimal price at or below evens pays nothing and is a placeholder, not a quote.
    return value if value > 1.0 else None


def load_quotes(directory: str) -> Tuple[List[MarketQuote], LoadCounts]:
    """Every usable two-runner closing quote in the cache directory, in date order.

    A missing directory is not an error: the harness runs without the odds and says so.
    """
    counts = LoadCounts()
    if not os.path.isdir(directory):
        logger.info("market odds: no cache directory at %s; the benchmark will report no coverage", directory)
        return [], counts
    grouped: Dict[str, List[Dict[str, str]]] = {}
    identity: Dict[str, Tuple[str, str, str]] = {}
    for path in sorted(glob.glob(os.path.join(directory, "*.csv"))):
        name = os.path.basename(path)
        found = _file_identity(name)
        if found is None:
            counts.unrecognised_files.append(name)
            logger.warning("market odds: %s matches no declared competition prefix; skipped", name)
            continue
        competition, gender = found
        counts.files.append(name)
        with open(path, newline="", encoding="utf-8-sig") as handle:
            for row in csv.DictReader(handle):
                counts.rows += 1
                market_id = f"{name}:{row.get('MARKET_ID', '')}"
                grouped.setdefault(market_id, []).append(row)
                identity[market_id] = (competition, gender, name)
    quotes: List[MarketQuote] = []
    for market_id, rows in grouped.items():
        competition, gender, _ = identity[market_id]
        prices = [_price(row.get(PRICED_AT, "")) for row in rows]
        if len(rows) != 2 or any(price is None for price in prices):
            counts.unusable += 1
            continue
        winners = [row["RUNNER_NAME"].strip() for row in rows if row.get("IS_WINNER") == "1"]
        quotes.append(
            MarketQuote(
                market_id=market_id,
                event_date=date.fromisoformat(rows[0]["EVENT_DATE"][:10]),
                gender=gender,
                competition=competition,
                runners=(rows[0]["RUNNER_NAME"].strip(), rows[1]["RUNNER_NAME"].strip()),
                back_prices=(float(prices[0]), float(prices[1])),
                winner=winners[0] if len(winners) == 1 else None,
            )
        )
    counts.quotes = len(quotes)
    quotes.sort(key=lambda quote: (quote.event_date, quote.market_id))
    logger.info("market odds: %d quotes from %d files in %s", counts.quotes, len(counts.files), directory)
    return quotes, counts


#: (team name, gender) -> the key the rating frame identifies that club by, or None when
#: the identity layer does not know the name. Both match sources provide one.
TeamKeyResolver = Callable[[str, str], Optional[str]]


def _resolve(resolver: TeamKeyResolver, known: frozenset, name: str, gender: str) -> Optional[str]:
    """The club key for a runner name: as written, else without a competition suffix.

    ``known`` is the set of club keys the frame actually holds, which is what makes the
    second attempt safe: a name is only stripped when the name as written resolves to
    nothing the archive has ever fielded.
    """
    stripped = RUNNER_NAME_SUFFIX.sub("", name).strip()
    for candidate in (name.strip(), stripped):
        if not candidate:
            continue
        key = resolver(candidate, gender)
        if key is not None and key in known:
            return key
    return None


def join_to_matches(
    quotes: Sequence[MarketQuote], frame: pd.DataFrame, resolver: TeamKeyResolver
) -> Tuple[pd.DataFrame, JoinCounts]:
    """Attach every quote it can to one match of the rating frame.

    The key is the match date and the pair of club identities, both sides resolved through
    the identity layer the frame itself is keyed by, so a rename or a men's/women's
    namesake cannot join to the wrong side. The returned frame has one row per joined
    match: its id, the market's P(team1 wins) after de-vigging, and the overround the
    de-vig removed.
    """
    counts = JoinCounts(quotes=len(quotes))
    index: Dict[Tuple[date, frozenset], List[Tuple[str, str, float]]] = {}
    for row in frame.itertuples(index=False):
        key = (row.match_date.date(), frozenset({row.team1, row.team2}))
        index.setdefault(key, []).append((row.match_id, row.team1, getattr(row, C.TARGET_COL)))
    known = frozenset(frame["team1"]) | frozenset(frame["team2"])
    records: List[Dict[str, object]] = []
    for quote in quotes:
        keys = [_resolve(resolver, known, name, quote.gender) for name in quote.runners]
        if any(key is None for key in keys) or keys[0] == keys[1]:
            counts.unknown_team += 1
            continue
        hits = index.get((quote.event_date, frozenset(keys)), [])
        if not hits:
            counts.no_match += 1
            continue
        if len(hits) > 1:
            counts.ambiguous += 1
            continue
        match_id, team1_key, outcome = hits[0]
        probabilities = quote.implied_probabilities()
        team1_probability = probabilities[0] if keys[0] == team1_key else probabilities[1]
        counts.joined += 1
        if quote.winner is not None and not pd.isna(outcome):
            market_says_team1_won = _resolve(resolver, known, quote.winner, quote.gender) == team1_key
            if market_says_team1_won != bool(outcome == 1.0):
                counts.label_disagreements += 1
        records.append(
            {
                "match_id": match_id,
                "market_probability": team1_probability,
                "market_overround": quote.overround,
                "competition": quote.competition,
            }
        )
    joined = pd.DataFrame.from_records(
        records, columns=["match_id", "market_probability", "market_overround", "competition"]
    )
    logger.info(
        "market odds: %d of %d quotes joined (%d no match, %d unknown team, %d ambiguous, %d label disagreements)",
        counts.joined,
        counts.quotes,
        counts.no_match,
        counts.unknown_team,
        counts.ambiguous,
        counts.label_disagreements,
    )
    return joined, counts


def _display_probabilities(displays: Sequence, rows: pd.DataFrame) -> Tuple[np.ndarray, np.ndarray]:
    """The two display arms on the same rows: the served probability (marginalised over
    the toss, averaged over the seeds) and the toss-aware one (read at the orientation
    that actually happened, which is the market's information set)."""
    served = np.mean([marginalised_probabilities(model, rows, C.DISPLAY_FEATURE_COLS) for model in displays], axis=0)
    features, _ = _xy(rows, C.DISPLAY_FEATURE_COLS)
    toss_aware = np.mean([model.predict_proba(features)[:, 1] for model in displays], axis=0)
    return served, toss_aware


def _scores(labels: np.ndarray, probabilities: np.ndarray) -> Dict[str, float]:
    return {
        "auc": float(roc_auc_score(labels, probabilities)),
        "brier": float(brier_score_loss(labels, probabilities)),
    }


def _bootstrap_delta_ci(labels: np.ndarray, market: np.ndarray, display: np.ndarray) -> Optional[List[float]]:
    """The 95 % interval of ``AUC(market) - AUC(display)``, resampling matches in pairs.

    Pairing matters: the two arms score the same matches, so their errors are correlated
    and two independent intervals would be far too wide to say anything.
    """
    rng = np.random.RandomState(BOOTSTRAP_SEED)
    n = len(labels)
    deltas: List[float] = []
    for _ in range(BOOTSTRAP_REPLICATES):
        pick = rng.randint(0, n, n)
        drawn = labels[pick]
        if len(np.unique(drawn)) < 2:
            continue
        deltas.append(roc_auc_score(drawn, market[pick]) - roc_auc_score(drawn, display[pick]))
    if not deltas:
        return None
    return [float(np.percentile(deltas, 2.5)), float(np.percentile(deltas, 97.5))]


def _window_report(labels: np.ndarray, market: np.ndarray, served: np.ndarray, toss_aware: np.ndarray) -> Dict:
    """One window's three arms and the two deltas that price the gap."""
    market_scores = _scores(labels, market)
    served_scores = _scores(labels, served)
    toss_scores = _scores(labels, toss_aware)
    return {
        "n": int(len(labels)),
        "market_auc": market_scores["auc"],
        "market_brier": market_scores["brier"],
        "display_auc_mean": served_scores["auc"],
        "display_brier_mean": served_scores["brier"],
        "display_toss_aware_auc": toss_scores["auc"],
        "display_toss_aware_brier": toss_scores["brier"],
        "market_minus_display_auc": market_scores["auc"] - served_scores["auc"],
        "market_minus_display_brier": market_scores["brier"] - served_scores["brier"],
        "market_minus_toss_aware_auc": market_scores["auc"] - toss_scores["auc"],
    }


@dataclass
class _Window:
    """One scored window's rows, kept until the format's pooled figure is computed."""

    label: str
    cutoff: str
    end: str
    labels: np.ndarray
    market: np.ndarray
    served: np.ndarray
    toss_aware: np.ndarray


class Benchmark:
    """Collects the market arm beside the display model as the harness walks its folds.

    The harness owns the folds, the models and the matches; this owns only the extra
    probability column, so nothing here can move a number any other gate reports. It is
    handed each window's fitted display models and evaluation rows, keeps the joined
    subset, and turns the lot into the report's ``market_benchmark`` section.
    """

    def __init__(self, joined: pd.DataFrame, load_counts: LoadCounts, join_counts: JoinCounts, cached_dir: str):
        self._probability = dict(zip(joined["match_id"], joined["market_probability"])) if len(joined) else {}
        self._overround = float(joined["market_overround"].mean()) if len(joined) else None
        self._load_counts = load_counts
        self._join_counts = join_counts
        self._cached_dir = cached_dir
        self._windows: Dict[str, List[_Window]] = {}
        self._matches_in_windows: Dict[str, int] = {}

    @property
    def has_quotes(self) -> bool:
        return bool(self._probability)

    def observe(
        self,
        format_code: str,
        label: str,
        cutoff: pd.Timestamp,
        end: pd.Timestamp,
        rows: pd.DataFrame,
        displays: Sequence,
    ) -> None:
        """Score one window: the harness's own evaluation rows, its own fitted display
        models, restricted to the matches a closing price was joined to."""
        if not len(displays) or not len(rows):
            return
        # The denominator of the format's coverage: matches the harness itself scored, so
        # a coverage share compares like with like.
        self._matches_in_windows[format_code] = self._matches_in_windows.get(format_code, 0) + int(len(rows))
        market = rows["match_id"].map(self._probability)
        scored = rows[market.notna()]
        if not len(scored):
            return
        served, toss_aware = _display_probabilities(displays, scored)
        self._windows.setdefault(format_code, []).append(
            _Window(
                label=label,
                cutoff=cutoff.date().isoformat(),
                end="" if end == pd.Timestamp.max else end.date().isoformat(),
                labels=scored[C.TARGET_COL].to_numpy(dtype=float),
                market=market[market.notna()].to_numpy(dtype=float),
                served=served,
                toss_aware=toss_aware,
            )
        )

    def _format_report(self, format_code: str) -> Dict:
        windows = self._windows.get(format_code, [])
        in_windows = self._matches_in_windows.get(format_code, 0)
        joined = int(sum(len(window.labels) for window in windows))
        entry: Dict[str, object] = {
            "matches_in_windows": in_windows,
            "matches_joined": joined,
            "joined_share": (joined / in_windows) if in_windows else 0.0,
            "folds": [],
            "pooled": None,
            "locked": {"n": 0, "note": "no joined closing price falls in the locked window"},
        }
        folds = [window for window in windows if window.label == "fold"]
        for window in folds:
            fold_entry: Dict[str, object] = {"cutoff": window.cutoff, "end": window.end, "n": int(len(window.labels))}
            if len(window.labels) >= MIN_SCORED_MATCHES and len(np.unique(window.labels)) == 2:
                fold_entry.update(_window_report(window.labels, window.market, window.served, window.toss_aware))
            else:
                fold_entry["skipped_reason"] = "too few joined matches in this window to score"
            entry["folds"].append(fold_entry)
        for window in windows:
            if window.label != "locked":
                continue
            locked: Dict[str, object] = {"n": int(len(window.labels)), "cutoff": window.cutoff}
            if len(window.labels) >= MIN_SCORED_MATCHES and len(np.unique(window.labels)) == 2:
                locked.update(_window_report(window.labels, window.market, window.served, window.toss_aware))
            else:
                locked["note"] = "too few joined matches in the locked window to score"
            entry["locked"] = locked
        if folds:
            labels = np.concatenate([window.labels for window in folds])
            market = np.concatenate([window.market for window in folds])
            served = np.concatenate([window.served for window in folds])
            toss_aware = np.concatenate([window.toss_aware for window in folds])
            if len(labels) >= MIN_SCORED_MATCHES and len(np.unique(labels)) == 2:
                pooled = _window_report(labels, market, served, toss_aware)
                pooled["market_minus_display_auc_ci95"] = _bootstrap_delta_ci(labels, market, served)
                pooled["market_minus_toss_aware_auc_ci95"] = _bootstrap_delta_ci(labels, market, toss_aware)
                entry["pooled"] = pooled
        return entry

    def report(self, format_codes: Iterable[str]) -> Dict:
        """The ``market_benchmark`` section, whether or not any odds were found."""
        formats = {format_code: self._format_report(format_code) for format_code in format_codes}
        scored = sum(int(entry["matches_joined"]) for entry in formats.values())
        return {
            "available": self.has_quotes,
            "source": {
                "name": SOURCE_NAME,
                "url": SOURCE_URL,
                "licence": SOURCE_LICENCE,
                "cached_dir": self._cached_dir,
                "priced_at": PRICED_AT_NOTE,
                **self._load_counts.as_dict(),
            },
            "devig": {"method": DEVIG_METHOD, "note": DEVIG_NOTE, "market_overround": self._overround},
            "join": {
                "quotes": self._join_counts.quotes,
                **self._join_counts.as_dict(),
                # A joined quote is not automatically a scored one: a season that ended
                # before the first walk-forward cutoff has no fold to be scored in. Without
                # this the join count and the per-format counts look like a contradiction.
                "quotes_joined_outside_scored_windows": max(self._join_counts.joined - scored, 0),
            },
            "bootstrap": {"replicates": BOOTSTRAP_REPLICATES, "seed": BOOTSTRAP_SEED},
            "formats": formats,
        }
