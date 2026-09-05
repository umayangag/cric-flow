"""Unit tests for the weather acquisition (X-2): the archive index and venue key, the
geocoding chooser and its curated table, the session-window rules, the ERA5 client,
reduction and cache, the four feature families, and the backfill's coverage and restore."""

from __future__ import annotations

import json
import os
from datetime import date

import httpx
import pytest

from ml.weather import archive, backfill, features, geocoding, sessions, venues

CRICSHEET_DIR = "cricsheet"


def _write_match(directory, match_id: str, **info) -> None:
    base = {
        "dates": ["2024-03-30"],
        "match_type": "T20",
        "gender": "male",
        "team_type": "club",
        "teams": ["Chennai Super Kings", "Mumbai Indians"],
        "venue": "Wankhede Stadium, Mumbai",
        "city": "Mumbai",
        "event": {"name": "Indian Premier League", "match_number": 12},
    }
    base.update(info)
    with open(os.path.join(directory, f"{match_id}.json"), "w") as fh:
        json.dump({"info": base}, fh)


@pytest.fixture
def cricsheet_dir(tmp_path):
    directory = tmp_path / CRICSHEET_DIR
    directory.mkdir()
    _write_match(directory, "1", event={"name": "Indian Premier League", "match_number": 11})
    _write_match(
        directory, "2", event={"name": "Indian Premier League", "match_number": 12}, venue="Eden Gardens", city=""
    )
    _write_match(
        directory,
        "3",
        dates=["2024-01-04", "2024-01-05", "2024-01-06"],
        match_type="Test",
        team_type="international",
        teams=["New Zealand", "South Africa"],
        venue="Seddon Park, Hamilton",
        city="Hamilton",
        event={"name": "South Africa in New Zealand Test Series"},
    )
    return str(directory)


@pytest.fixture
def window_day() -> archive.DayWeather:
    return archive.DayWeather(
        venue_key="wankhede stadium mumbai",
        day=date(2024, 3, 30),
        timezone="Asia/Kolkata",
        temperature_c=[20.0 + h for h in range(24)],
        relative_humidity=[50.0 + h for h in range(24)],
        precipitation_mm=[0.5] * 24,
        prior_precipitation_mm=[1.0, 0.0, 0.0, 2.0, 0.0, 0.0, 4.0],
    )


# --- venues -----------------------------------------------------------------------------


def test_venue_key_folds_case_accents_and_punctuation() -> None:
    """Two spellings of one ground share a key."""
    assert venues.venue_key("M. Chinnaswamy Stadium, Bengaluru") == "m chinnaswamy stadium bengaluru"
    assert venues.venue_key("Moara Vlăsiei  Cricket Ground") == venues.venue_key("moara vlasiei cricket ground")


def test_read_archive_indexes_fixtures_and_venue_facts(cricsheet_dir) -> None:
    """Fixtures carry the first day; venues collect cities, dates and country votes."""
    fixtures, facts = venues.read_archive(cricsheet_dir)
    by_id = {f.match_id: f for f in fixtures}
    assert by_id["3"].date == date(2024, 1, 4)
    assert by_id["3"].international and by_id["3"].match_number is None
    assert by_id["1"].match_number == 11
    hamilton = facts[venues.venue_key("Seddon Park, Hamilton")]
    assert hamilton.city_hint() == "Hamilton"
    assert hamilton.countries()[:2] == ["NZ", "ZA"]
    assert facts[venues.venue_key("Wankhede Stadium, Mumbai")].countries() == ["IN"]


def test_city_hint_falls_back_to_the_venue_name_then_to_nothing() -> None:
    assert venues.VenueFacts(venue="Lord's, London").city_hint() == "London"
    assert venues.VenueFacts(venue="Holkar Stadium").city_hint() == ""


def test_dates_by_venue_is_the_distinct_sorted_first_days(cricsheet_dir) -> None:
    fixtures, _ = venues.read_archive(cricsheet_dir)
    assert venues.dates_by_venue(fixtures)["eden gardens"] == [date(2024, 3, 30)]


def test_team_and_competition_countries_are_looked_up_case_insensitively() -> None:
    assert venues.team_countries("West Indies") == venues.WEST_INDIES
    assert venues.team_countries("Mars") == ()
    assert venues.competition_countries("Big Bash League") == ("AU",)
    assert venues.competition_countries("Unheard Of Cup") == ()


# --- geocoding --------------------------------------------------------------------------


def _candidate(name, country, population, **kw) -> geocoding.Candidate:
    return geocoding.Candidate(name, 1.0, 2.0, country, country, "UTC", population, **kw)


class _FakeGeocoder:
    def __init__(self, answers) -> None:
        self.answers = answers
        self.queries = []

    def search(self, name):
        self.queries.append(name)
        return self.answers.get(name, [])


def test_choose_prefers_the_voted_country_over_population() -> None:
    """Hamilton is in New Zealand when New Zealand's sides play there."""
    candidates = [_candidate("Hamilton", "CA", 500_000), _candidate("Hamilton", "NZ", 150_000)]
    assert geocoding.choose(candidates, ["NZ"]).country_code == "NZ"
    assert geocoding.choose(candidates, []).country_code == "CA"
    assert geocoding.choose([], ["NZ"]) is None


def test_locate_uses_the_city_hint_then_the_venue_parts_then_records_unmappable() -> None:
    facts = venues.VenueFacts(venue="Seddon Park, Hamilton")
    facts.cities["Hamilton"] += 1
    facts.country_votes["NZ"] += 3
    client = _FakeGeocoder({"Hamilton": [_candidate("Hamilton", "NZ", 1)]})
    located = geocoding.locate(facts, client)
    assert located.mapped and located.query == "Hamilton" and located.note == ""
    fallback = geocoding.locate(
        venues.VenueFacts(venue="Cobham Oval, Whangarei"),
        _FakeGeocoder({"Whangarei": [_candidate("Whangarei", "NZ", 1)]}),
    )
    assert fallback.mapped and fallback.note.startswith("no country vote")
    nowhere = geocoding.locate(venues.VenueFacts(venue="Holkar Stadium"), _FakeGeocoder({}))
    assert nowhere.status == geocoding.STATUS_UNMAPPABLE and not nowhere.mapped


def test_locate_notes_a_country_the_archive_did_not_vote_for() -> None:
    facts = venues.VenueFacts(venue="Dubai International Cricket Stadium")
    facts.country_votes["PK"] += 1
    client = _FakeGeocoder({"Dubai International Cricket Stadium": [_candidate("Dubai", "AE", 1)]})
    assert geocoding.locate(facts, client).note == "country not among the archive's votes"


def test_locations_csv_round_trips_and_missing_file_is_empty(tmp_path) -> None:
    path = str(tmp_path / "geo.csv")
    assert geocoding.read_locations(path) == {}
    row = geocoding.VenueLocation("Lord's", "lord s", geocoding.STATUS_MAPPED, latitude=51.5, longitude=-0.1)
    geocoding.write_locations(path, {"lord s": row})
    back = geocoding.read_locations(path)["lord s"]
    assert back.latitude == pytest.approx(51.5) and back.mapped
    unmapped = geocoding.VenueLocation("X", "x", geocoding.STATUS_UNMAPPABLE)
    geocoding.write_locations(path, {"x": unmapped})
    assert geocoding.read_locations(path)["x"].latitude is None


def test_geocode_missing_asks_only_about_venues_without_a_row() -> None:
    facts = {"a": venues.VenueFacts(venue="A"), "b": venues.VenueFacts(venue="B")}
    locations = {"a": geocoding.VenueLocation("A", "a", geocoding.STATUS_MAPPED, latitude=0.0, longitude=0.0)}
    client = _FakeGeocoder({"B": [_candidate("B", "IN", 1)]})
    assert geocoding.geocode_missing(facts, locations, client) == 1
    assert client.queries == ["B"] and locations["b"].mapped


def test_open_meteo_geocoding_parses_results() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.params["name"] == "Mumbai"
        return httpx.Response(
            200,
            json={
                "results": [
                    {
                        "name": "Mumbai",
                        "latitude": 19.07,
                        "longitude": 72.88,
                        "country_code": "IN",
                        "country": "India",
                        "timezone": "Asia/Kolkata",
                        "population": 12_000_000,
                        "admin1": "Maharashtra",
                    },
                    {"name": "no coords"},
                ]
            },
        )

    client = geocoding.OpenMeteoGeocoding(httpx.Client(transport=httpx.MockTransport(handler)), pause_seconds=0.0)
    found = client.search("Mumbai")
    assert len(found) == 1 and found[0].admin1 == "Maharashtra" and found[0].population == 12_000_000


# --- sessions ---------------------------------------------------------------------------


def _fixture(**kw) -> venues.Fixture:
    base = dict(
        match_id="m",
        venue="V",
        date=date(2024, 3, 30),
        match_type="T20",
        gender="male",
        competition="",
        match_number=None,
        teams=("A", "B"),
        international=False,
    )
    base.update(kw)
    return venues.Fixture(**base)


@pytest.mark.parametrize(
    "kw, country, ranks, expected",
    [
        (dict(match_type="Test", international=True), "GB", (0, 1, 0), (11, False, "first_class")),
        (dict(match_type="MDM"), "IN", (0, 1, 0), (10, False, "first_class")),
        (dict(match_type="ODI", international=True), "IN", (0, 1, 0), (13, True, "odi_men:IN")),
        (dict(match_type="ODI", international=True), "GB", (0, 1, 0), (11, False, "odi_men:GB")),
        (dict(match_type="ODI", international=True, gender="female"), "IN", (0, 1, 0), (10, False, "odi_women")),
        (dict(match_type="ODM"), "IN", (0, 1, 0), (9, False, "one_day_domestic:IN")),
        (
            dict(competition="Indian Premier League"),
            "IN",
            (0, 1, 0),
            (19, True, "t20_league:indian premier league:single"),
        ),
        (
            dict(competition="Indian Premier League"),
            "IN",
            (0, 2, 0),
            (15, False, "t20_league:indian premier league:double_0"),
        ),
        (
            dict(competition="Indian Premier League"),
            "IN",
            (1, 2, 0),
            (19, True, "t20_league:indian premier league:double_1"),
        ),
        (dict(competition="ICC Men's T20 World Cup", international=True), "US", (0, 1, 0), (19, True, "t20i_icc:men")),
        (
            dict(competition="ICC Women's T20 World Cup", international=True, gender="female"),
            "AE",
            (0, 2, 0),
            (15, False, "t20i_icc:women"),
        ),
        (
            dict(competition="ICC Men's T20 World Cup Qualifier", international=True),
            "NA",
            (0, 1, 1),
            (14, False, "t20i_associate:slot_1"),
        ),
        (dict(international=True, gender="female"), "AU", (0, 1, 0), (14, False, "t20i_women")),
        (dict(international=True), "AU", (0, 1, 0), (19, True, "t20i_men:AU")),
        (dict(competition="Vitality Blast"), "GB", (0, 9, 0), (14, False, "t20_domestic:weekend")),
        (
            dict(competition="Vitality Blast", date=date(2024, 3, 29)),
            "GB",
            (0, 9, 0),
            (18, True, "t20_domestic:weekday"),
        ),
        (dict(competition="Nepal T20"), "NP", (0, 3, 2), (18, True, "t20_domestic_day:slot_2")),
        (dict(match_type="ODM"), "", (0, 1, 0), (10, False, "one_day_domestic:??")),
        (dict(match_type="XYZ"), "IN", (0, 1, 0), (10, False, "unknown_type:XYZ")),
    ],
)
def test_infer_applies_the_documented_rule(kw, country, ranks, expected) -> None:
    """Each rule returns its start hour, its day/night flag and its own id."""
    window = sessions.infer(_fixture(**kw), country, *ranks)
    assert (window.start_hour, window.night, window.rule) == expected


def test_assign_ranks_double_headers_by_match_number_and_censuses_the_rules() -> None:
    late = _fixture(match_id="late", competition="Indian Premier League", match_number=2)
    early = _fixture(match_id="early", competition="Indian Premier League", match_number=1, venue="Elsewhere")
    windows = sessions.assign([late, early], {"v": "IN", "elsewhere": "IN"})
    assert windows["early"].start_hour == 15 and windows["late"].start_hour == 19
    assert sessions.census(windows) == {
        "t20_league:indian premier league:double_0": 1,
        "t20_league:indian premier league:double_1": 1,
    }


# --- archive ----------------------------------------------------------------------------


def test_clusters_groups_days_within_the_gap() -> None:
    days = [date(2024, 1, 1), date(2024, 1, 20), date(2024, 6, 1), date(2024, 1, 1)]
    assert archive.clusters(days, gap_days=45) == [[date(2024, 1, 1), date(2024, 1, 20)], [date(2024, 6, 1)]]


def _response(days, missing_day=None) -> archive.ArchiveResponse:
    from datetime import timedelta

    first = min(days) - timedelta(days=archive.PRIOR_DAYS)
    span = [(first + timedelta(days=i)) for i in range((max(days) - first).days + 1)]
    hourly_time = [f"{d.isoformat()}T{h:02d}:00" for d in span for h in range(24)]
    temperature = [None if (missing_day and t.startswith(missing_day.isoformat())) else 20.0 for t in hourly_time]
    return archive.ArchiveResponse(
        timezone="Asia/Kolkata",
        hourly_time=hourly_time,
        hourly={
            "temperature_2m": temperature,
            "relative_humidity_2m": [60.0] * len(hourly_time),
            "precipitation": [0.1] * len(hourly_time),
        },
        daily_time=[d.isoformat() for d in span],
        daily={"precipitation_sum": [float(i) for i in range(len(span))]},
    )


def test_reduce_days_keeps_the_day_hours_and_the_prior_week_and_records_a_miss() -> None:
    days = [date(2024, 3, 30), date(2024, 3, 31)]
    entries = archive.reduce_days("v", _response(days, missing_day=date(2024, 3, 31)), days, "t0")
    hit, miss = entries
    assert isinstance(hit, archive.DayWeather) and len(hit.temperature_c) == 24
    assert hit.prior_precipitation_mm == [0.0, 1.0, 2.0, 3.0, 4.0, 5.0, 6.0]
    assert isinstance(miss, archive.Miss) and miss.reason == "no hourly readings in the archive"


def test_cache_round_trips_hits_and_misses_and_skips_a_torn_line(tmp_path, window_day) -> None:
    path = str(tmp_path / "cache.jsonl")
    cache = archive.WeatherCache(path)
    cache.append([window_day, archive.Miss("v", date(2024, 1, 1), "why")])
    with open(path, "a") as fh:
        fh.write('{"venue": "torn"')
    reopened = archive.WeatherCache(path)
    assert len(reopened) == 2 and reopened.counts() == {"days": 1, "misses": 1}
    day = reopened.get(window_day.venue_key, window_day.day)
    assert day.relative_humidity[0] == 50 and day.temperature_c[23] == pytest.approx(43.0)
    assert isinstance(reopened.get("v", date(2024, 1, 1)), archive.Miss)
    assert reopened.get("v", date(2024, 1, 2)) is None


class _FakeArchive:
    def __init__(self) -> None:
        self.calls = []

    def fetch(self, latitude, longitude, start, end):
        self.calls.append((start, end))
        from datetime import timedelta

        days = [start + timedelta(days=archive.PRIOR_DAYS), end]
        return _response(days)


def test_backfill_fetches_one_call_per_cluster_and_skips_unmapped_and_recent_days(tmp_path) -> None:
    cache = archive.WeatherCache(str(tmp_path / "c.jsonl"))
    locations = {
        "v": geocoding.VenueLocation("V", "v", geocoding.STATUS_MAPPED, latitude=1.0, longitude=2.0),
        "u": geocoding.VenueLocation("U", "u", geocoding.STATUS_UNMAPPABLE),
    }
    days = {
        "v": [date(2024, 3, 30), date(2024, 3, 31), date(2024, 9, 4)],
        "u": [date(2024, 3, 30)],
        "w": [date(2024, 1, 1)],
    }
    client = _FakeArchive()
    counts = archive.backfill(locations, days, cache, client, today=date(2024, 9, 5))
    assert counts == {"venues": 1, "calls": 1, "days_fetched": 2, "misses": 0, "too_recent": 1, "unmapped_days": 2}
    assert client.calls == [(date(2024, 3, 23), date(2024, 3, 31))]
    assert archive.backfill(locations, days, cache, client, today=date(2024, 9, 5))["calls"] == 0


def _archive_transport(statuses):
    calls = []

    def handler(request: httpx.Request) -> httpx.Response:
        status, body = statuses[len(calls)]
        calls.append(request)
        return (
            httpx.Response(status, json=body) if body is not None else httpx.Response(status, text="<html>busy</html>")
        )

    return httpx.MockTransport(handler), calls


def test_open_meteo_archive_parses_a_response(monkeypatch) -> None:
    monkeypatch.setattr(archive.time, "sleep", lambda s: None)
    body = {
        "timezone": "Asia/Kolkata",
        "hourly": {"time": ["2024-03-30T00:00"], "temperature_2m": [21.0]},
        "daily": {"time": ["2024-03-30"], "precipitation_sum": [0.0]},
    }
    transport, calls = _archive_transport([(200, body)])
    client = archive.OpenMeteoArchive(httpx.Client(transport=transport))
    response = client.fetch(1.0, 2.0, date(2024, 3, 23), date(2024, 3, 30))
    assert response.timezone == "Asia/Kolkata" and response.hourly["temperature_2m"] == [21.0]
    assert calls[0].url.params["timezone"] == "auto" and client.calls == 1


def test_open_meteo_archive_retries_transient_errors_and_raises_on_the_daily_limit(monkeypatch) -> None:
    monkeypatch.setattr(archive.time, "sleep", lambda s: None)
    transport, _ = _archive_transport(
        [(503, {"reason": "busy"}), (200, {"timezone": "UTC", "hourly": {"time": []}, "daily": {"time": []}})]
    )
    assert (
        archive.OpenMeteoArchive(httpx.Client(transport=transport))
        .fetch(1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2))
        .timezone
        == "UTC"
    )
    transport, _ = _archive_transport([(429, {"reason": "Daily API request limit exceeded"})])
    with pytest.raises(archive.DailyLimitReached):
        archive.OpenMeteoArchive(httpx.Client(transport=transport)).fetch(1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2))
    transport, calls = _archive_transport(
        [(200, None), (200, {"timezone": "UTC", "hourly": {"time": []}, "daily": {"time": []}})]
    )
    assert (
        archive.OpenMeteoArchive(httpx.Client(transport=transport))
        .fetch(1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2))
        .timezone
        == "UTC"
    )
    assert len(calls) == 2
    transport, _ = _archive_transport([(200, None)] * 4)
    with pytest.raises(RuntimeError, match="after 4 attempts"):
        archive.OpenMeteoArchive(httpx.Client(transport=transport)).fetch(1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2))
    transport, _ = _archive_transport([(400, {"reason": "bad"})])
    with pytest.raises(httpx.HTTPStatusError):
        archive.OpenMeteoArchive(httpx.Client(transport=transport)).fetch(1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2))


# --- features ---------------------------------------------------------------------------


def test_feature_row_reads_only_the_hours_before_the_start(window_day) -> None:
    """A 19:00 night start reads 16-18h; rain is yesterday plus today's hours before 19."""
    row = features.feature_row(sessions.SessionWindow(19, True, "r"), window_day)
    assert row[features.KNOWN_COL] == 1.0 and row[features.NIGHT_COL] == 1.0
    assert row["wx_pre_temp_c"] == pytest.approx(37.0)
    assert row["wx_pre_humidity"] == pytest.approx(67.0)
    assert row["wx_dew_proxy"] == pytest.approx(0.67)
    assert row["wx_rain_prior_day_mm"] == pytest.approx(4.0 + 19 * 0.5)
    assert row["wx_rain_prior_week_mm"] == pytest.approx(7.0)


def test_feature_row_is_the_unknown_category_without_readings(window_day) -> None:
    day_window = sessions.SessionWindow(10, False, "r")
    assert features.feature_row(day_window, None) == {**{c: 0.0 for c in features.WEATHER_COLS}}
    window_day.temperature_c = [None] * 24
    row = features.feature_row(day_window, window_day)
    assert row[features.KNOWN_COL] == 0.0 and row["wx_dew_proxy"] == 0.0


# --- backfill ---------------------------------------------------------------------------


def _locations_for(cricsheet_dir):
    _, facts = venues.read_archive(cricsheet_dir)
    return {
        key: geocoding.VenueLocation(
            f.venue, key, geocoding.STATUS_MAPPED, latitude=1.0, longitude=2.0, timezone="UTC", place="P", country="C"
        )
        for key, f in facts.items()
        if key != "eden gardens"
    }


def test_coverage_counts_mapped_days_misses_and_unanswered_per_format(cricsheet_dir, tmp_path, window_day) -> None:
    fixtures, _ = venues.read_archive(cricsheet_dir)
    locations = _locations_for(cricsheet_dir)
    cache = archive.WeatherCache(str(tmp_path / "c.jsonl"))
    cache.append([window_day])
    windows = sessions.assign(fixtures, {k: "IN" for k in locations})
    report = backfill.coverage(fixtures, locations, cache, windows)
    assert report["matches"]["T20"] == {
        "matches": 2,
        "venue_mapped": 1,
        "weather_day": 1,
        "venue_unmapped": 1,
        "night": 0,
        "features_known": 1,
    }
    assert report["matches"]["Test"] == {
        "matches": 1,
        "venue_mapped": 1,
        "unanswered": 1,
        "night": 0,
        "features_known": 0,
    }
    assert report["matches"]["all"]["matches"] == 3 and report["venues"]["mapped"] == 2
    backfill.print_report(report)


class _FakeCursor:
    def __init__(self) -> None:
        self.executed = []
        self.rowcount = 1

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def execute(self, sql, params=None):
        self.executed.append((sql, params))

    def fetchall(self):
        return [(7, "Wankhede Stadium, Mumbai"), (8, "Wankhede  Stadium, Mumbai")]

    def executemany(self, sql, rows):
        self.executed.append((sql, list(rows)))


class _FakeConnection:
    def __init__(self) -> None:
        self.cur = _FakeCursor()
        self.committed = False

    def cursor(self):
        return self.cur

    def commit(self):
        self.committed = True


def test_write_to_db_updates_every_venue_row_sharing_the_key_and_upserts_days(tmp_path, window_day) -> None:
    cache = archive.WeatherCache(str(tmp_path / "c.jsonl"))
    cache.append([window_day, archive.Miss("wankhede stadium mumbai", date(2024, 1, 1), "no")])
    locations = {
        "wankhede stadium mumbai": geocoding.VenueLocation(
            "Wankhede Stadium, Mumbai",
            "wankhede stadium mumbai",
            geocoding.STATUS_MAPPED,
            latitude=19.0,
            longitude=72.0,
        ),
        "nowhere": geocoding.VenueLocation("Nowhere", "nowhere", geocoding.STATUS_MAPPED, latitude=0.0, longitude=0.0),
        "unmapped": geocoding.VenueLocation("U", "unmapped", geocoding.STATUS_UNMAPPABLE),
    }
    connection = _FakeConnection()
    counts = backfill.write_to_db(connection, locations, cache)
    assert counts == {"venues_updated": 2, "weather_rows": 2, "venues_without_row": 1}
    upsert_rows = connection.cur.executed[-1][1]
    assert sorted(r[0] for r in upsert_rows) == [7, 8] and upsert_rows[0][8] == archive.SOURCE_LICENSE
    assert connection.committed


def test_run_offline_serves_the_files_and_writes_the_report(cricsheet_dir, tmp_path, monkeypatch) -> None:
    geocoding_path = str(tmp_path / "geo.csv")
    geocoding.write_locations(geocoding_path, _locations_for(cricsheet_dir))
    cache_path = str(tmp_path / "c.jsonl")
    report_path = str(tmp_path / "r.json")
    fake = _FakeConnection()
    monkeypatch.setattr("ml.db.get_db_connection", lambda: fake)
    assert (
        backfill.main(
            [
                "--cricsheet-dir",
                cricsheet_dir,
                "--geocoding",
                geocoding_path,
                "--cache",
                cache_path,
                "--offline",
                "--to-db",
                "--report",
                "--report-json",
                report_path,
            ]
        )
        == 0
    )
    with open(report_path) as fh:
        report = json.load(fh)
    assert report["database"]["weather_rows"] == 0 and report["matches"]["all"]["unanswered"] == 2
    assert not os.path.exists(cache_path)
