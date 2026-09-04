"""Unit tests for X-4's market benchmark (ml.xi.market): the yardstick, never a feature."""

from __future__ import annotations

import csv
from datetime import date
from pathlib import Path
from typing import Dict, List, Optional, Sequence

import numpy as np
import pandas as pd
import pytest

from ml.xi import contract as C
from ml.xi import market

HEADER = [
    "EVENT_DATE",
    "PATH",
    "EVENT_ID",
    "MARKET_TYPE",
    "MARKET_ID",
    "MARKET_NAME",
    "SELECTION_ID",
    "RUNNER_NAME",
    "RUNNER_STATUS",
    "IS_WINNER",
    "HOME_TEAM",
    "AWAY_TEAM",
    market.PRICED_AT,
]


def _write_season(directory: Path, name: str, markets: Sequence[Dict]) -> Path:
    """One cached season file, in the source's own column layout."""
    path = directory / name
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(HEADER)
        for entry in markets:
            for runner, price, is_winner in entry["runners"]:
                writer.writerow(
                    [
                        f"{entry['date']} 00:00:00.000 Z",
                        "Twenty20 Big Bash",
                        "1",
                        "MATCH_ODDS",
                        entry["market_id"],
                        "Match Odds",
                        "1",
                        runner,
                        "ACTIVE",
                        "1" if is_winner else "0",
                        "",
                        "",
                        price,
                    ]
                )
    return path


def _two_runner_market(market_id: str, day: str, home: str, away: str, prices=("2.0", "2.0"), winner: int = 0) -> Dict:
    return {
        "market_id": market_id,
        "date": day,
        "runners": [(home, prices[0], winner == 0), (away, prices[1], winner == 1)],
    }


def _frame(rows: Sequence[Dict]) -> pd.DataFrame:
    """A match frame with every display feature present, so the display arms can be read."""
    rng = np.random.RandomState(0)
    frame = pd.DataFrame(rows)
    frame["match_date"] = pd.to_datetime(frame["match_date"])
    columns = list(C.DISPLAY_FEATURE_COLS) + [
        f"{prefix}_{stem}" for prefix in ("t1", "t2", "d") for stem in C.SIDE_FEATURE_STEMS
    ]
    for column in columns:
        frame[column] = rng.normal(size=len(frame))
    frame["team_h2h"] = 0.5
    return frame


def _match(match_id: str, day: str, team1: str, team2: str, team1_wins: float) -> Dict:
    return {
        "match_id": match_id,
        "match_date": day,
        "format_code": "T20",
        "team1": team1,
        "team2": team2,
        C.TARGET_COL: team1_wins,
    }


def _resolver(known: Dict) -> market.TeamKeyResolver:
    return lambda name, gender: known.get((name, gender))


class _ColumnModel:
    """A display model that reads one feature column, so both arms are deterministic."""

    def __init__(self, column_index: int = 0, weight: float = 1.0):
        self.column_index = column_index
        self.weight = weight

    def predict_proba(self, x: np.ndarray) -> np.ndarray:
        p = 1.0 / (1.0 + np.exp(-self.weight * x[:, self.column_index]))
        return np.column_stack([1.0 - p, p])


# --- the source: what a cached directory yields --------------------------------------


def test_load_quotes_reads_a_season_and_names_its_competition_and_gender(tmp_path: Path) -> None:
    """The file's prefix, not the runner names, says which competition and gender it is."""
    _write_season(tmp_path, "WBBL11_Match_Odds.csv", [_two_runner_market("1", "2025-11-09", "Perth W", "Sixers W")])

    quotes, counts = market.load_quotes(str(tmp_path))

    assert len(quotes) == 1
    assert quotes[0].gender == C.GENDER_FEMALE
    assert quotes[0].competition == "Women's Big Bash League"
    assert quotes[0].event_date == date(2025, 11, 9)
    assert counts.quotes == 1 and counts.unrecognised_files == []


def test_load_quotes_counts_a_file_whose_competition_is_not_declared(tmp_path: Path) -> None:
    """A file matching no declared prefix has no known gender, so it is skipped and named."""
    _write_season(tmp_path, "IPL2025_Match_Odds.csv", [_two_runner_market("1", "2025-04-01", "A", "B")])

    quotes, counts = market.load_quotes(str(tmp_path))

    assert quotes == []
    assert counts.unrecognised_files == ["IPL2025_Match_Odds.csv"]


def test_load_quotes_drops_a_market_with_an_unusable_price(tmp_path: Path) -> None:
    _write_season(
        tmp_path,
        "BBL15_Match_Odds.csv",
        [
            _two_runner_market("1", "2025-12-14", "Heat", "Stars", prices=("1.0", "2.0")),
            _two_runner_market("2", "2025-12-15", "Heat", "Sixers", prices=("1.6", "2.6")),
        ],
    )

    quotes, counts = market.load_quotes(str(tmp_path))

    assert [quote.market_id.endswith(":2") for quote in quotes] == [True]
    assert counts.unusable == 1


def test_load_quotes_on_a_missing_directory_is_empty_rather_than_an_error(tmp_path: Path) -> None:
    quotes, counts = market.load_quotes(str(tmp_path / "absent"))

    assert quotes == [] and counts.quotes == 0


def test_cache_dir_prefers_the_argument_then_the_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv(market.CACHE_DIR_ENV, "/from/env")

    assert market.cache_dir("/explicit") == "/explicit"
    assert market.cache_dir(None) == "/from/env"


# --- the de-vig ----------------------------------------------------------------------


def test_proportional_devig_normalises_the_pair_and_reports_the_overround() -> None:
    """Two 2.0 quotes imply 0.5 each with no overround; a 1.5/2.5 pair is normalised."""
    even = market.MarketQuote("m", date(2025, 1, 1), "male", "c", ("a", "b"), (2.0, 2.0), None)
    skewed = market.MarketQuote("m", date(2025, 1, 1), "male", "c", ("a", "b"), (1.5, 2.5), None)

    assert even.implied_probabilities() == pytest.approx((0.5, 0.5))
    assert even.overround == pytest.approx(1.0)
    assert sum(skewed.implied_probabilities()) == pytest.approx(1.0)
    assert skewed.implied_probabilities()[0] == pytest.approx((1 / 1.5) / (1 / 1.5 + 1 / 2.5))
    assert skewed.overround == pytest.approx(1 / 1.5 + 1 / 2.5)


# --- the join ------------------------------------------------------------------------


def _quote(day: str, runners, prices=(1.5, 3.0), winner: Optional[str] = None) -> market.MarketQuote:
    return market.MarketQuote(
        market_id=f"f:{day}",
        event_date=date.fromisoformat(day),
        gender=C.GENDER_MALE,
        competition="Big Bash League",
        runners=runners,
        back_prices=prices,
        winner=winner,
    )


def test_join_orients_the_market_probability_to_the_side_that_batted_first() -> None:
    frame = _frame([_match("m1", "2025-12-14", "k-heat", "k-stars", 1.0)])
    quotes = [_quote("2025-12-14", ("Stars", "Heat"))]
    resolver = _resolver({("Heat", "male"): "k-heat", ("Stars", "male"): "k-stars"})

    joined, counts = market.join_to_matches(quotes, frame, resolver)

    assert counts.joined == 1
    # The quote lists Stars first at 1.5; team1 is Heat, quoted at 3.0, so its share is the smaller.
    assert joined.loc[0, "market_probability"] == pytest.approx((1 / 3.0) / (1 / 1.5 + 1 / 3.0))


def test_join_strips_a_competition_suffix_only_when_the_name_as_written_is_unknown() -> None:
    """WBBL seasons spell a club "Perth Scorchers W"; the club itself is the identity."""
    frame = _frame([_match("m1", "2025-11-09", "k-perth", "k-sixers", 0.0)])
    quotes = [_quote("2025-11-09", ("Perth Scorchers W", "Sydney Sixers W"))]
    resolver = _resolver({("Perth Scorchers", "male"): "k-perth", ("Sydney Sixers", "male"): "k-sixers"})

    joined, counts = market.join_to_matches(quotes, frame, resolver)

    assert counts.joined == 1 and len(joined) == 1


def test_join_counts_a_team_the_identity_layer_does_not_know_rather_than_guessing() -> None:
    frame = _frame([_match("m1", "2025-12-14", "k-heat", "k-stars", 1.0)])
    quotes = [_quote("2025-12-14", ("Heat", "Somebody Else"))]
    resolver = _resolver({("Heat", "male"): "k-heat"})

    joined, counts = market.join_to_matches(quotes, frame, resolver)

    assert counts.unknown_team == 1 and counts.joined == 0 and joined.empty


def test_join_counts_a_quote_whose_date_matches_no_fixture() -> None:
    frame = _frame([_match("m1", "2025-12-14", "k-heat", "k-stars", 1.0)])
    quotes = [_quote("2025-12-15", ("Heat", "Stars"))]
    resolver = _resolver({("Heat", "male"): "k-heat", ("Stars", "male"): "k-stars"})

    _, counts = market.join_to_matches(quotes, frame, resolver)

    assert counts.no_match == 1 and counts.joined == 0


def test_join_refuses_a_quote_that_two_fixtures_answer_to() -> None:
    """A double-header of the same pair on one day is not resolvable by date and teams."""
    frame = _frame(
        [
            _match("m1", "2025-12-14", "k-heat", "k-stars", 1.0),
            _match("m2", "2025-12-14", "k-stars", "k-heat", 0.0),
        ]
    )
    quotes = [_quote("2025-12-14", ("Heat", "Stars"))]
    resolver = _resolver({("Heat", "male"): "k-heat", ("Stars", "male"): "k-stars"})

    _, counts = market.join_to_matches(quotes, frame, resolver)

    assert counts.ambiguous == 1 and counts.joined == 0


def test_join_counts_a_winner_the_two_sources_disagree_about() -> None:
    """The integrity check on the whole mapping: a disagreement means a bad join."""
    frame = _frame([_match("m1", "2025-12-14", "k-heat", "k-stars", 1.0)])
    quotes = [_quote("2025-12-14", ("Heat", "Stars"), winner="Stars")]
    resolver = _resolver({("Heat", "male"): "k-heat", ("Stars", "male"): "k-stars"})

    _, counts = market.join_to_matches(quotes, frame, resolver)

    assert counts.joined == 1 and counts.label_disagreements == 1


def test_join_accepts_a_winner_both_sources_agree_about() -> None:
    frame = _frame([_match("m1", "2025-12-14", "k-heat", "k-stars", 1.0)])
    quotes = [_quote("2025-12-14", ("Heat", "Stars"), winner="Heat")]
    resolver = _resolver({("Heat", "male"): "k-heat", ("Stars", "male"): "k-stars"})

    _, counts = market.join_to_matches(quotes, frame, resolver)

    assert counts.label_disagreements == 0


# --- the benchmark ------------------------------------------------------------------


def _scored_frame(n: int) -> pd.DataFrame:
    rows = [_match(f"m{i}", f"2025-12-{(i % 28) + 1:02d}", "k-heat", "k-stars", float(i % 2 == 0)) for i in range(n)]
    return _frame(rows)


def _benchmark_over(frame: pd.DataFrame, probabilities: Sequence[float]) -> Dict:
    joined = pd.DataFrame(
        {
            "match_id": list(frame["match_id"]),
            "market_probability": list(probabilities),
            "market_overround": [1.01] * len(frame),
            "competition": ["Big Bash League"] * len(frame),
        }
    )
    benchmark = market.Benchmark(joined, market.LoadCounts(), market.JoinCounts(joined=len(joined)), "/cache")
    benchmark.observe("T20", "fold", pd.Timestamp("2025-12-01"), pd.Timestamp("2026-01-01"), frame, [_ColumnModel()])
    return benchmark.report(C.FORMAT_CODES)


def test_benchmark_scores_all_three_arms_on_the_same_matches() -> None:
    frame = _scored_frame(40)
    # A market that knows the answer: it must beat both display arms outright.
    perfect = [0.9 if wins else 0.1 for wins in frame[C.TARGET_COL]]

    report = _benchmark_over(frame, perfect)
    pooled = report["formats"]["T20"]["pooled"]

    assert pooled["n"] == 40
    assert pooled["market_auc"] == pytest.approx(1.0)
    assert pooled["market_minus_display_auc"] == pytest.approx(1.0 - pooled["display_auc_mean"])
    assert pooled["market_minus_toss_aware_auc"] == pytest.approx(1.0 - pooled["display_toss_aware_auc"])
    assert pooled["market_minus_display_auc_ci95"][0] <= pooled["market_minus_display_auc"]


def test_benchmark_states_the_joined_coverage_beside_every_number() -> None:
    """A benchmark over part of a format says which part, on its face."""
    frame = _scored_frame(40)
    joined = pd.DataFrame(
        {
            "match_id": list(frame["match_id"])[:10],
            "market_probability": [0.5] * 10,
            "market_overround": [1.01] * 10,
            "competition": ["Big Bash League"] * 10,
        }
    )
    benchmark = market.Benchmark(joined, market.LoadCounts(), market.JoinCounts(joined=10), "/cache")
    benchmark.observe("T20", "fold", pd.Timestamp("2025-12-01"), pd.Timestamp("2026-01-01"), frame, [_ColumnModel()])

    entry = benchmark.report(C.FORMAT_CODES)["formats"]["T20"]

    assert entry["matches_in_windows"] == 40
    assert entry["matches_joined"] == 10
    assert entry["joined_share"] == pytest.approx(0.25)


def test_benchmark_declines_to_score_a_window_with_too_few_joined_matches() -> None:
    frame = _scored_frame(market.MIN_SCORED_MATCHES - 1)

    report = _benchmark_over(frame, [0.5] * len(frame))
    fold = report["formats"]["T20"]["folds"][0]

    assert fold["n"] == market.MIN_SCORED_MATCHES - 1
    assert fold["skipped_reason"] == "too few joined matches in this window to score"
    assert report["formats"]["T20"]["pooled"] is None


def test_benchmark_without_any_odds_reports_no_coverage_rather_than_nothing() -> None:
    benchmark = market.Benchmark(pd.DataFrame(columns=["match_id"]), market.LoadCounts(), market.JoinCounts(), "/cache")

    report = benchmark.report(C.FORMAT_CODES)

    assert report["available"] is False
    assert set(report["formats"]) == set(C.FORMAT_CODES)
    for format_code in C.FORMAT_CODES:
        assert report["formats"][format_code]["joined_share"] == 0.0
        assert report["formats"][format_code]["pooled"] is None


def test_benchmark_names_the_source_the_de_vig_and_the_join_rule() -> None:
    """Every number the section prints must be traceable to where it came from."""
    report = market.Benchmark(
        pd.DataFrame(columns=["match_id"]), market.LoadCounts(), market.JoinCounts(), "/cache"
    ).report(C.FORMAT_CODES)

    assert report["source"]["name"] == market.SOURCE_NAME
    assert report["source"]["cached_dir"] == "/cache"
    assert "never committed" in report["source"]["licence"]
    assert report["devig"]["method"] == market.DEVIG_METHOD
    assert "identity layer" in report["join"]["key"]


def test_the_join_counts_account_for_every_loaded_quote() -> None:
    """No quote may be dropped quietly: the outcomes have to add up to what was loaded."""
    frame = _frame([_match("m1", "2025-12-14", "k-heat", "k-stars", 1.0)])
    quotes = [
        _quote("2025-12-14", ("Heat", "Stars")),
        _quote("2025-12-15", ("Heat", "Stars")),
        _quote("2025-12-14", ("Heat", "Nobody")),
    ]
    resolver = _resolver({("Heat", "male"): "k-heat", ("Stars", "male"): "k-stars"})

    _, counts = market.join_to_matches(quotes, frame, resolver)

    assert counts.joined + counts.no_match + counts.unknown_team + counts.ambiguous == counts.quotes


# --- the rule that makes this a yardstick and not a feature --------------------------

#: Everything that builds a feature, fits a model, or serves a prediction. None of it may
#: reach the odds: a model that consumed them would be predicting the market rather than
#: the cricket, and could not answer for a fixture nobody has priced.
FEATURE_AND_SERVING_MODULES: List[str] = [
    "contract.py",
    "builder.py",
    "ratings.py",
    "rows.py",
    "train.py",
    "retrain.py",
    "performance.py",
    "simulator.py",
    "optimizer.py",
    "asof.py",
    "sequence.py",
    "store.py",
    "sources.py",
]


@pytest.mark.parametrize("module_name", FEATURE_AND_SERVING_MODULES)
def test_nothing_that_builds_or_serves_a_model_imports_the_odds(module_name: str) -> None:
    source = (Path(market.__file__).parent / module_name).read_text()

    assert "ml.xi.market" not in source
    assert "import market" not in source


def test_no_feature_column_is_derived_from_the_market() -> None:
    every_column = set(C.DISPLAY_FEATURE_COLS) | set(C.XI_FEATURE_COLS) | set(C.PLAYER_VECTOR_KEYS)

    assert not [column for column in every_column if "market" in column or "odds" in column]
