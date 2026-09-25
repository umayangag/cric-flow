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
    assert dict(hamilton.country_votes) == {"NZ": 1, "ZA": 1}
    assert dict(facts[venues.venue_key("Wankhede Stadium, Mumbai")].country_votes) == {"IN": 1}


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


@pytest.mark.parametrize(
    "event, expected",
    [
        ("Zimbabwe tour of Australia", ()),  # a tour is not a competition anyone is at home in
        ("Zimbabwe in Bangladesh ODI Series", ()),
        ("ACC Twenty20 Cup", ()),  # the Asian Cricket Council's, never England's
        ("CSA T20 Challenge", ("ZA",)),  # matched whole, without the table's old trailing space
        ("ECB Women's One-Day Cup", ("GB",)),
        ("Vitality Blast Women", ("GB",)),
        ("The Marsh Cup", ("AU",)),
    ],
)
def test_a_competition_needle_never_matches_a_country_a_tour_is_named_after(event, expected) -> None:
    """DATA-03: "zimbabwe" was a needle, so every one of the 443 fixtures whose event name
    carries the country voted ZW for wherever it was played -- Townsville, Bloemfontein,
    Hyderabad. A needle names a competition, matched as a whole phrase, or nothing."""
    assert venues.competition_countries(event) == expected


def test_no_competition_needle_is_an_international_side_the_archive_names() -> None:
    """The structural guard behind DATA-03: a needle that is also a team name votes for
    that team's country on every tour it plays, home or away."""
    needles = {needle for needle, _ in venues.COMPETITION_COUNTRIES}

    assert needles & set(venues.TEAM_COUNTRIES) == set()


def test_a_tour_votes_only_for_the_sides_that_played_it(tmp_path) -> None:
    """DATA-03's verified case: Zimbabwe's tour of Australia at Townsville used to vote ZW
    twice -- once for the side, once for the event name -- and out-vote the host."""
    directory = tmp_path / CRICSHEET_DIR
    directory.mkdir()
    _write_match(
        directory,
        "1",
        team_type="international",
        teams=["Zimbabwe", "Australia"],
        venue="Tony Ireland Stadium, Townsville",
        city="Townsville",
        event={"name": "Zimbabwe tour of Australia"},
    )

    _, facts = venues.read_archive(str(directory))

    assert dict(facts["tony ireland stadium townsville"].country_votes) == {"ZW": 1, "AU": 1}


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
    assert geocoding.choose(candidates, {"NZ": 3}).country_code == "NZ"
    assert geocoding.choose(candidates, {}).country_code == "CA"
    assert geocoding.choose([], {"NZ": 3}) is None


@pytest.mark.parametrize(
    "top_votes, candidate_votes, beyond_chance",
    [
        (102, 2, True),  # M Chinnaswamy Stadium: India against the Sindh homonym
        (105, 33, True),  # Shere Bangla National Stadium: Bangladesh against the Punjab homonym
        (70, 50, True),  # Providence Stadium: Guyana against Rhode Island
        (15, 3, True),  # National Cricket Stadium, Grenada: the West Indies against Wales
        (11, 3, True),  # Coolidge Cricket Ground: Antigua against Arizona
        (15, 7, False),  # Sportpark Het Schootsveld: a Dutch ground England's sides visit
        (6, 2, False),  # Botswana Cricket Association Oval 2: not evidence either way
        (21, 18, False),
        (2, 1, False),
        (1, 1, False),
        (5, 0, True),  # Grenada's fifteen votes against Bermuda's none: silence is a conflict here
        (4, 0, False),  # Sharjah's four against the Emirates' none: it is not
    ],
)
def test_lead_beyond_chance_is_the_one_sided_sign_test(top_votes, candidate_votes, beyond_chance) -> None:
    """A lead is a conflict only when a fair coin would produce it under CONFLICT_P_VALUE."""
    assert geocoding.lead_beyond_chance(top_votes, candidate_votes) is beyond_chance


def test_choose_refuses_a_homonym_in_a_country_the_top_vote_out_votes_beyond_chance() -> None:
    """DATA-01: the only place named Bangalore the geocoder returned is Bangalore Town,
    Sindh, and Pakistan carries 2 of the archive's votes to India's 102 -- the candidate is
    a conflict, not a placement, and the query yields nothing."""
    bangalore_town = _candidate("Bangalore Town", "PK", 20_000, admin1="Sindh")
    mirpur = _candidate("Mīrpur", "IN", 40_000, admin1="Punjab")
    assert geocoding.choose([bangalore_town], {"IN": 102, "PK": 2}) is None
    assert geocoding.choose([mirpur], {"BD": 105, "IN": 33}) is None
    assert geocoding.support("PK", {"IN": 102, "PK": 2}) == geocoding.SUPPORT_CONFLICT


def test_choose_keeps_a_minority_country_the_top_vote_does_not_exclude_and_notes_it() -> None:
    """Six Namibian votes to two Botswanan ones is not evidence against Gaborone, and the
    row says how the votes fell."""
    gaborone = _candidate("Gaborone", "BW", 230_000)
    votes = {"NA": 6, "BW": 2}
    assert geocoding.choose([gaborone], votes) is gaborone
    assert geocoding.support("BW", votes) == geocoding.SUPPORT_MINORITY
    assert geocoding.support_note("BW", votes) == "country not the archive's top vote (NA 6 to 2)"


def test_choose_ranks_a_voted_for_country_over_one_nobody_voted_for() -> None:
    """Against a weak top vote, silence is not a conflict -- but a country that did get a
    vote outranks it, whatever the populations."""
    gaborone = _candidate("Gaborone", "BW", 230_000)
    homonym = _candidate("Gaborone", "ZZ", 900_000)
    votes = {"NA": 4, "BW": 2}
    assert geocoding.choose([homonym, gaborone], votes) is gaborone
    assert geocoding.support("ZZ", votes) == geocoding.SUPPORT_UNVOTED
    assert geocoding.support_note("ZZ", votes) == geocoding.NOTE_UNVOTED


def test_choose_refuses_an_unvoted_country_when_the_top_vote_is_strong() -> None:
    """DATA-01, second query: with Wales refused, 'St George's' answered Bermuda, which
    nobody voted for -- fifteen West Indian votes against none is the same conflict."""
    bermuda = _candidate("Saint George's Parish", "BM", 2_000)
    votes = {"GD": 15, "JM": 15, "GB": 3}
    assert geocoding.choose([bermuda], votes) is None
    assert geocoding.support("BM", votes) == geocoding.SUPPORT_CONFLICT
    assert geocoding.support("BM", {"NP": 4, "GB": 2}) == geocoding.SUPPORT_UNVOTED


def test_support_treats_a_tied_top_vote_as_the_top_vote() -> None:
    """The West Indies vote for every territory at once, so Bridgetown ties Kingston at the
    top and is supported without a note."""
    votes = {"JM": 48, "BB": 48, "GY": 48, "US": 28}
    assert geocoding.support("BB", votes) == geocoding.SUPPORT_TOP
    assert geocoding.support_note("BB", votes) == ""
    assert geocoding.top_vote(votes) == ("BB", 48)


def test_locate_records_a_venue_whose_every_candidate_conflicts_as_unmappable_with_the_places() -> None:
    """The reason is on the row: which top vote refused which places."""
    facts = venues.VenueFacts(venue="M Chinnaswamy Stadium")
    facts.cities["Bangalore"] += 90
    facts.country_votes.update({"IN": 102, "PK": 2})
    client = _FakeGeocoder({"Bangalore": [_candidate("Bangalore Town", "PK", 20_000, admin1="Sindh")]})
    located = geocoding.locate(facts, client)
    assert located.status == geocoding.STATUS_UNMAPPABLE and not located.mapped
    assert located.query == "Bangalore | M Chinnaswamy Stadium"
    assert located.note == "every candidate conflicts with the archive's top vote (IN 102): Bangalore Town (PK)"
    assert located.countries_voted == "IN:102 PK:2"


def test_votes_column_round_trips_and_refuses_a_row_without_counts() -> None:
    votes = {"BD": 105, "ZW": 37, "IN": 33, "AE": 5}
    text = geocoding.format_votes(votes)
    assert text == "BD:105 ZW:37 IN:33 AE:5"
    assert geocoding.parse_votes(text) == votes
    assert geocoding.parse_votes("") == {}
    with pytest.raises(ValueError, match="carries no count"):
        geocoding.parse_votes("IN AU GB")


CURATED_TABLE = os.path.join(os.path.dirname(__file__), "..", "..", "reference-data", "venue-geocoding.csv")


def _unsupported_rows(locations) -> list:
    """Every mapped row that is neither hand-placed nor placed where its own votes allow,
    or whose note does not say how its votes fall short."""
    problems = []
    for location in locations.values():
        if location.note.startswith("hand-curated") or not location.mapped:
            continue
        votes = geocoding.parse_votes(location.countries_voted)
        kind = geocoding.support(location.country_code, votes)
        expected_note = geocoding.support_note(location.country_code, votes)
        if kind == geocoding.SUPPORT_CONFLICT or location.note != expected_note:
            problems.append(f"{location.venue!r} -> {location.country_code} [{kind}] note={location.note!r}")
    return problems


def test_curated_table_places_every_row_where_its_votes_allow_or_says_why() -> None:
    """DATA-01 over the whole curated table: no row sits in a country the archive's top
    vote out-votes beyond chance, and every row not in the top-voted country carries the
    note that says so -- unless a hand placed it and wrote why."""
    locations = geocoding.read_locations(CURATED_TABLE)
    assert len(locations) > 800
    assert _unsupported_rows(locations) == []


def test_curation_summary_classifies_by_note() -> None:
    """DATA-09: each placement kind's note maps to its own count, and a note matching
    none of them is refused rather than silently mis-counted."""
    locations = {
        "top": geocoding.VenueLocation("Top", "top", geocoding.STATUS_MAPPED, note=""),
        "minority": geocoding.VenueLocation(
            "Minority", "minority", geocoding.STATUS_MAPPED, note=f"{geocoding.NOTE_MINORITY_PREFIX}IN 5 to 3)"
        ),
        "unvoted": geocoding.VenueLocation("Unvoted", "unvoted", geocoding.STATUS_MAPPED, note=geocoding.NOTE_UNVOTED),
        "no_votes": geocoding.VenueLocation(
            "NoVotes", "no_votes", geocoding.STATUS_MAPPED, note=geocoding.NOTE_NO_VOTES
        ),
        "hand": geocoding.VenueLocation(
            "Hand", "hand", geocoding.STATUS_MAPPED, note="hand-curated: the ground's city, country pinned"
        ),
        "unmapped": geocoding.VenueLocation("Gone", "unmapped", geocoding.STATUS_UNMAPPABLE, note="no place found"),
    }

    summary = geocoding.curation_summary(locations)

    assert summary == geocoding.CurationSummary(
        total=5, top_vote=1, minority_vote=1, unvoted=1, no_votes=1, hand_curated=1
    )


def test_curation_summary_refuses_a_note_it_cannot_classify() -> None:
    locations = {"x": geocoding.VenueLocation("X", "x", geocoding.STATUS_MAPPED, note="something new")}

    with pytest.raises(ValueError, match="matches no known placement kind"):
        geocoding.curation_summary(locations)


def test_curation_summary_matches_the_documented_counts() -> None:
    """DATA-09: `reference-data/README.md` and `contracts/system-map.json` quote this
    table's breakdown by hand. Derived here from the file itself, so a doc that drifts from
    it again (as it did before DATA-01 corrected 88 hand-curated rows to 112) fails a test
    rather than sitting silently wrong."""
    locations = geocoding.read_locations(CURATED_TABLE)

    summary = geocoding.curation_summary(locations)

    assert summary == geocoding.CurationSummary(
        total=892, top_vote=673, minority_vote=59, unvoted=44, no_votes=4, hand_curated=112
    )


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
    assert fallback.mapped and fallback.note == geocoding.NOTE_NO_VOTES
    nowhere = geocoding.locate(venues.VenueFacts(venue="Holkar Stadium"), _FakeGeocoder({}))
    assert nowhere.status == geocoding.STATUS_UNMAPPABLE and not nowhere.mapped
    assert nowhere.note == geocoding.NOTE_NO_PLACE


def test_locate_holds_a_country_centroid_and_takes_a_place_in_the_same_country() -> None:
    """DATA-02: Cricsheet names "Barbados" as the city beside the Kensington Oval, so the
    geocoder answered with the country and the row sat at its centroid -- 10 km from the
    ground, and from where the other three spellings of the same ground were placed. The
    centroid is held, the venue name's own parts are asked, and Bridgetown replaces it."""
    facts = venues.VenueFacts(venue="Kensington Oval, Bridgetown")
    facts.cities["Barbados"] += 57
    facts.country_votes["BB"] += 57
    client = _FakeGeocoder(
        {
            "Barbados": [_candidate("Barbados", "BB", 287_000)],
            "Bridgetown": [_candidate("Bridgetown", "BB", 98_000, admin1="Saint Michael")],
        }
    )

    located = geocoding.locate(facts, client)

    assert located.query == "Bridgetown" and located.place == "Bridgetown"
    assert located.admin1 == "Saint Michael" and located.note == ""
    assert client.queries == ["Barbados", "Bridgetown"]


def test_locate_keeps_a_country_centroid_when_the_venue_name_answers_another_country() -> None:
    """The other half of DATA-02: "Lords, St David's Cricket Club Ground" is in Bermuda,
    and its own name answers La Verne, California. A place replaces the centroid only when
    it is in the same country, so the centroid stays."""
    facts = venues.VenueFacts(venue="Lords, St David's Cricket Club Ground")
    facts.cities["Bermuda"] += 4
    client = _FakeGeocoder(
        {
            "Bermuda": [_candidate("Bermuda", "BM", 64_000)],
            "Lords": [_candidate("La Verne", "US", 31_000, admin1="California")],
        }
    )

    located = geocoding.locate(facts, client)

    assert located.place == "Bermuda" and located.country_code == "BM"
    assert "Lords" in client.queries


def test_is_country_centroid_only_when_the_query_was_the_country_name() -> None:
    """A place inside a country is never a centroid, and neither is a country whose name
    the query did not ask for -- which is how "Rwandarugali" stops out-ranking "Rwanda"."""
    barbados = _candidate("Barbados", "BB", 287_000)
    bridgetown = _candidate("Bridgetown", "BB", 98_000, admin1="Saint Michael")

    assert geocoding.is_country_centroid(barbados, "Barbados") is True
    assert geocoding.is_country_centroid(barbados, "Bridgetown") is False
    assert geocoding.is_country_centroid(bridgetown, "Bridgetown") is False


def test_every_spelling_of_one_ground_is_placed_at_one_set_of_coordinates() -> None:
    """DATA-02, on the committed table: the key deliberately does not merge spellings --
    "County Ground" is nine different grounds in this archive, and a rule that folded the
    first comma-part would make them one. What it must not do is place one ground in two
    places, which is what a country centroid did to the Kensington Oval and the Queen's
    Park Oval."""
    locations = geocoding.read_locations(CURATED_TABLE)

    for prefix in ("kensington oval", "queen s park oval"):
        placements = {
            (round(loc.latitude, 4), round(loc.longitude, 4))
            for key, loc in locations.items()
            if key == prefix or key.startswith(prefix + " ")
        }
        assert len(placements) == 1, f"{prefix}: {placements}"
    county = {key for key in locations if key == "county ground" or key.startswith("county ground ")}
    assert len(county) == 9


def test_locate_notes_a_country_the_archive_did_not_vote_for() -> None:
    facts = venues.VenueFacts(venue="Dubai International Cricket Stadium")
    facts.country_votes["PK"] += 1
    client = _FakeGeocoder({"Dubai International Cricket Stadium": [_candidate("Dubai", "AE", 1)]})
    assert geocoding.locate(facts, client).note == geocoding.NOTE_UNVOTED


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
            (15, False, "t20_league:indian premier league:first_of_day"),
        ),
        (
            dict(competition="Indian Premier League"),
            "IN",
            (1, 2, 0),
            (19, True, "t20_league:indian premier league:later_in_day"),
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
        "t20_league:indian premier league:first_of_day": 1,
        "t20_league:indian premier league:later_in_day": 1,
    }


@pytest.mark.parametrize(
    "rule, documented",
    [
        ("odi_men:IN", True),
        ("odi_women", False),
        ("one_day_domestic:GB", True),
        ("t20i_men:AU", True),
        ("t20i_women", False),
        ("t20i_associate:slot_0", False),
        ("t20i_associate:slot_1", False),
        ("t20_domestic_day:slot_2", False),
        ("t20_domestic:weekday", True),
        ("t20_league:indian premier league:single", True),
        ("t20i_icc:women", True),
        ("first_class", True),
        ("unknown_type:XYZ", True),
    ],
)
def test_has_documented_start_hour_excludes_the_rank_and_blanket_rules(rule, documented) -> None:
    """DATA-08: a rule that assigns one hardcoded hour to every country alike for a
    bilateral international format (``odi_women``, ``t20i_women``), or reads an hour off a
    rotation keyed by the fixture's rank at its venue (``t20i_associate``,
    ``t20_domestic_day``), is not a documented single start hour."""
    assert sessions.has_documented_start_hour(rule) is documented


def test_night_share_documented_only_drops_the_rule_artefact_rows() -> None:
    """DATA-08: the 35.2 % night share the census implies mixes real evidence with rule
    artefacts. Restricting to documented rules changes the share, because the excluded
    rows are not a random sample of the day/night split."""
    windows = {
        "m1": sessions.SessionWindow(13, True, "odi_men:IN"),
        "m2": sessions.SessionWindow(11, False, "odi_men:GB"),
        "m3": sessions.SessionWindow(10, False, "odi_women"),
        "m4": sessions.SessionWindow(18, True, "t20i_associate:slot_2"),  # placed night by rank alone
    }

    assert sessions.night_share(windows) == pytest.approx(0.5)  # m1 and m4 of four
    assert sessions.night_share(windows, documented_only=True) == pytest.approx(0.5)  # m1 of {m1, m2}


def test_night_share_is_none_when_no_window_qualifies() -> None:
    windows = {"m1": sessions.SessionWindow(10, False, "odi_women")}

    assert sessions.night_share(windows, documented_only=True) is None


# --- archive ----------------------------------------------------------------------------


def test_clusters_groups_days_within_the_gap() -> None:
    days = [date(2024, 1, 1), date(2024, 1, 20), date(2024, 6, 1), date(2024, 1, 1)]
    assert archive.clusters(days, gap_days=45) == [[date(2024, 1, 1), date(2024, 1, 20)], [date(2024, 6, 1)]]


def _response(days, missing_day=None, zone="Asia/Kolkata") -> archive.ArchiveResponse:
    """One call's answer, stamped in UTC as the corrected client asks for it: the cluster,
    its prior week and a day's margin at each end, each hour's temperature its own index so
    a reading can be traced back to the UTC hour it came from."""
    from datetime import timedelta

    first = min(days) - timedelta(days=archive.PRIOR_DAYS + archive.UTC_MARGIN_DAYS)
    last = max(days) + timedelta(days=archive.UTC_MARGIN_DAYS)
    span = [(first + timedelta(days=i)) for i in range((last - first).days + 1)]
    hourly_time = [f"{d.isoformat()}T{h:02d}:00" for d in span for h in range(24)]
    blank = set(archive.local_hour_keys(missing_day, zone)) if missing_day else set()
    temperature = [None if t in blank else float(i) for i, t in enumerate(hourly_time)]
    return archive.ArchiveResponse(
        hourly_time=hourly_time,
        hourly={
            "temperature_2m": temperature,
            "relative_humidity_2m": [60.0] * len(hourly_time),
            "precipitation": [0.1] * len(hourly_time),
        },
    )


def test_reduce_days_keeps_the_day_hours_and_the_prior_week_and_records_a_miss() -> None:
    """The day's 24 local hours in the venue's zone, the local daily rain totals of the
    week before, and a miss for a day the archive answered nothing for."""
    days = [date(2024, 3, 30), date(2024, 3, 31)]

    entries = archive.reduce_days("v", _response(days, missing_day=date(2024, 3, 31)), days, "t0", "Asia/Kolkata")

    hit, miss = entries
    assert isinstance(hit, archive.DayWeather) and len(hit.temperature_c) == 24
    assert hit.timezone == "Asia/Kolkata"
    assert hit.prior_precipitation_mm == [pytest.approx(2.4)] * 7
    assert isinstance(miss, archive.Miss) and miss.reason == "no hourly readings in the archive"


def test_local_hour_keys_follow_the_zone_across_a_daylight_saving_transition() -> None:
    """DATA-04: New Zealand's clocks go back on 7 April 2024, so a January match day's
    midnight is 11:00 UTC the day before and a June one's is 12:00 -- the offset the zone
    was on *that day*, not the one it is on when the call is made."""
    summer = archive.local_hour_keys(date(2024, 1, 10), "Pacific/Auckland")
    winter = archive.local_hour_keys(date(2024, 6, 10), "Pacific/Auckland")

    assert summer[0] == "2024-01-09T11:00" and summer[23] == "2024-01-10T10:00"
    assert winter[0] == "2024-06-09T12:00" and winter[23] == "2024-06-10T11:00"
    # Asia/Kolkata's +5:30 is floored to the whole hour ERA5 is gridded on.
    assert archive.local_hour_keys(date(2024, 3, 30), "Asia/Kolkata")[0] == "2024-03-29T19:00"


def test_reduce_days_reads_each_day_at_its_own_offset_within_one_call() -> None:
    """DATA-04's failure, pinned: one call covering both sides of a DST transition used to
    stamp every day with a single offset, so the January day was an hour out. Each day is
    placed on its own offset here, and the hours it holds are the UTC hours that offset
    names -- a January midnight from 11:00 UTC, a June one from 12:00."""
    days = [date(2024, 1, 10), date(2024, 6, 10)]

    summer, winter = archive.reduce_days("v", _response(days), days, "t0", "Pacific/Auckland")

    def utc_index(label: str) -> float:
        return float(_response(days).hourly_time.index(label))

    assert summer.temperature_c[0] == utc_index("2024-01-09T11:00")
    assert winter.temperature_c[0] == utc_index("2024-06-09T12:00")
    assert winter.temperature_c[0] - summer.temperature_c[0] != 24 * (date(2024, 6, 10) - date(2024, 1, 10)).days


def test_reduce_days_refuses_a_permanent_miss_when_the_call_answered_nothing() -> None:
    """DATA-06: a day the archive has no readings for is a miss only when the response
    answered for a neighbour. A response empty throughout is a bad 200, and recording it
    would make a permanent miss a restore never re-asks."""
    days = [date(2024, 3, 30)]
    empty = _response(days)
    empty.hourly["temperature_2m"] = [None] * len(empty.hourly_time)

    with pytest.raises(ValueError, match="refusing to record a permanent miss"):
        archive.reduce_days("v", empty, days, "t0", "Asia/Kolkata")


def test_cache_round_trips_hits_and_misses_and_skips_a_torn_line(tmp_path, window_day) -> None:
    path = str(tmp_path / "cache.jsonl")
    cache = archive.WeatherCache(path)
    cache.append([window_day, archive.Miss("v", date(2024, 1, 1), "why")])
    with open(path, "a") as fh:
        fh.write('{"venue": "torn"')
    reopened = archive.WeatherCache(path)
    assert len(reopened) == 2 and reopened.counts() == {"days": 1, "misses": 1}
    assert reopened.skipped_lines == 1
    day = reopened.get(window_day.venue_key, window_day.day)
    assert day.relative_humidity[0] == 50 and day.temperature_c[23] == pytest.approx(43.0)
    assert isinstance(reopened.get("v", date(2024, 1, 1)), archive.Miss)
    assert reopened.get("v", date(2024, 1, 2)) is None


def test_cache_refuses_an_unparsable_line_that_is_not_the_last(tmp_path, window_day, caplog) -> None:
    """DATA-06: a torn final line costs one cluster and is logged; a bad line anywhere else
    means the file was corrupted after it was written, and dropping it silently would turn
    the days it held into gaps a later run re-asks at whatever coordinates it then holds."""
    path = str(tmp_path / "cache.jsonl")
    cache = archive.WeatherCache(path)
    cache.append([window_day])
    with open(path, "a") as fh:
        fh.write('{"venue": "torn"\n')
        fh.write(json.dumps(archive.Miss("v", date(2024, 1, 1), "why").to_line()) + "\n")

    with pytest.raises(ValueError, match="line 2 could not be read"):
        archive.WeatherCache(path)


class _FakeArchive:
    def __init__(self) -> None:
        self.calls = []

    def fetch(self, latitude, longitude, start, end):
        self.calls.append((start, end))
        from datetime import timedelta

        days = [start + timedelta(days=archive.PRIOR_DAYS + archive.UTC_MARGIN_DAYS), end - timedelta(days=1)]
        return _response(days)


def test_backfill_fetches_one_call_per_cluster_and_skips_unmapped_and_recent_days(tmp_path) -> None:
    cache = archive.WeatherCache(str(tmp_path / "c.jsonl"))
    locations = {
        "v": geocoding.VenueLocation(
            "V", "v", geocoding.STATUS_MAPPED, latitude=1.0, longitude=2.0, timezone="Asia/Kolkata"
        ),
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
    assert client.calls == [(date(2024, 3, 22), date(2024, 4, 1))]
    assert archive.backfill(locations, days, cache, client, today=date(2024, 9, 5))["calls"] == 0


def test_backfill_skips_a_mapped_venue_with_no_zone_to_read_its_hours_on() -> None:
    """Coordinates without an IANA zone cannot be reduced to local hours, so the days count
    as unmapped rather than being fetched into a day whose hours mean nothing (DATA-04)."""
    locations = {"v": geocoding.VenueLocation("V", "v", geocoding.STATUS_MAPPED, latitude=1.0, longitude=2.0)}
    cache = archive.WeatherCache("")
    client = _FakeArchive()

    counts = archive.backfill(locations, {"v": [date(2024, 3, 30)]}, cache, client, today=date(2024, 9, 5))

    assert counts["unmapped_days"] == 1 and counts["calls"] == 0 and client.calls == []


def _archive_transport(statuses):
    calls = []

    def handler(request: httpx.Request) -> httpx.Response:
        status, body = statuses[len(calls)]
        calls.append(request)
        return (
            httpx.Response(status, json=body) if body is not None else httpx.Response(status, text="<html>busy</html>")
        )

    return httpx.MockTransport(handler), calls


def test_open_meteo_archive_asks_in_utc_and_parses_a_response(monkeypatch) -> None:
    """DATA-04: the call asks for UTC, never the service's own "auto", whose offset is the
    one the zone happens to be on at the moment of the call."""
    monkeypatch.setattr(archive.time, "sleep", lambda s: None)
    body = {"hourly": {"time": ["2024-03-30T00:00"], "temperature_2m": [21.0]}}
    transport, calls = _archive_transport([(200, body)])

    client = archive.OpenMeteoArchive(httpx.Client(transport=transport))
    response = client.fetch(1.0, 2.0, date(2024, 3, 23), date(2024, 3, 30))

    assert response.hourly_time == ["2024-03-30T00:00"] and response.hourly["temperature_2m"] == [21.0]
    assert calls[0].url.params["timezone"] == "UTC" and client.calls == 1
    assert "daily" not in calls[0].url.params


def test_open_meteo_archive_retries_transient_errors_and_raises_on_the_daily_limit(monkeypatch) -> None:
    monkeypatch.setattr(archive.time, "sleep", lambda s: None)
    ok = {"hourly": {"time": ["2024-01-01T00:00"], "temperature_2m": [1.0]}}
    transport, _ = _archive_transport([(503, {"reason": "busy"}), (200, ok)])
    assert archive.OpenMeteoArchive(httpx.Client(transport=transport)).fetch(
        1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2)
    ).hourly_time == ["2024-01-01T00:00"]
    transport, _ = _archive_transport([(429, {"reason": "Daily API request limit exceeded"})])
    with pytest.raises(archive.DailyLimitReached):
        archive.OpenMeteoArchive(httpx.Client(transport=transport)).fetch(1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2))
    transport, calls = _archive_transport([(200, None), (200, ok)])
    assert archive.OpenMeteoArchive(httpx.Client(transport=transport)).fetch(
        1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2)
    ).hourly_time == ["2024-01-01T00:00"]
    assert len(calls) == 2
    transport, _ = _archive_transport([(200, None)] * 4)
    with pytest.raises(RuntimeError, match="after 4 attempts"):
        archive.OpenMeteoArchive(httpx.Client(transport=transport)).fetch(1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2))
    slept = []
    monkeypatch.setattr(archive.time, "sleep", lambda s: slept.append(s))
    transport, calls = _archive_transport([(429, {"reason": "Hourly API request limit exceeded"}), (200, ok)])
    assert archive.OpenMeteoArchive(httpx.Client(transport=transport)).fetch(
        1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2)
    ).hourly_time == ["2024-01-01T00:00"]
    assert len(calls) == 2 and archive.HOURLY_LIMIT_PAUSE_SECONDS in slept
    transport, _ = _archive_transport([(400, {"reason": "bad"})])
    with pytest.raises(httpx.HTTPStatusError):
        archive.OpenMeteoArchive(httpx.Client(transport=transport)).fetch(1.0, 2.0, date(2024, 1, 1), date(2024, 1, 2))


# --- features ---------------------------------------------------------------------------


def test_feature_row_reads_the_morning_before_play_can_start(window_day) -> None:
    """Whatever the inferred start, the same-day windows end at 08:00: humidity and
    temperature are the 05-07h means, rain is yesterday plus the match day's hours 0-7."""
    row = features.feature_row(sessions.SessionWindow(19, True, "r"), window_day)
    assert row[features.KNOWN_COL] == 1.0 and row[features.NIGHT_COL] == 1.0
    assert row["wx_pre_temp_c"] == pytest.approx(26.0)
    assert row["wx_pre_humidity"] == pytest.approx(56.0)
    assert row["wx_dew_proxy"] == pytest.approx(0.56)
    assert row["wx_rain_prior_day_mm"] == pytest.approx(4.0 + 8 * 0.5)
    assert row["wx_rain_prior_week_mm"] == pytest.approx(7.0)
    assert features.feature_row(sessions.SessionWindow(10, False, "r"), window_day)["wx_pre_temp_c"] == pytest.approx(
        26.0
    )


def test_feature_row_reads_no_hour_an_earlier_actual_start_could_have_put_in_play(window_day) -> None:
    """DATA-05: an IPL match placed at the 19:00 single norm that in fact began at the
    double-header's 15:30. Rain at 15-18h fell during play and humidity at 16-18h was
    read during play; neither reaches a column. Only the morning's rain does."""
    window_day.precipitation_mm = [0.0] * 24
    for hour in range(15, 19):
        window_day.precipitation_mm[hour] = 6.0
    window_day.precipitation_mm[3] = 0.5
    window_day.relative_humidity = [60.0] * 24
    for hour in range(16, 19):
        window_day.relative_humidity[hour] = 100.0

    row = features.feature_row(sessions.SessionWindow(19, True, "t20_league:indian premier league:single"), window_day)

    assert row["wx_rain_prior_day_mm"] == pytest.approx(4.0 + 0.5)
    assert row["wx_pre_humidity"] == pytest.approx(60.0)
    assert row["wx_dew_proxy"] == pytest.approx(0.60)
    assert row["wx_pre_temp_c"] == pytest.approx(26.0)


def _every_inferable_window():
    countries = sorted(
        set(sessions.ODI_START_BY_COUNTRY)
        | set(sessions.ONE_DAY_DOMESTIC_START_BY_COUNTRY)
        | set(sessions.FIRST_CLASS_START_BY_COUNTRY)
        | set(sessions.T20I_START_BY_COUNTRY)
        | set(sessions.T20_DOMESTIC_NIGHT_COUNTRIES)
        | {"", "US", "NP"}
    )
    competitions = [needle for needle, _ in sessions.T20_LEAGUE_NORMS] + ["ICC Men's T20 World Cup", "Plain Cup"]
    ranks = [(0, 1, 0), (0, 2, 0), (1, 2, 1), (2, 3, 2)]
    for match_type in ("Test", "MDM", "ODI", "ODM", "T20", "IT20", "XYZ"):
        for gender in ("male", "female"):
            for international in (False, True):
                for day in (date(2024, 3, 29), date(2024, 3, 30)):  # a Friday, a Saturday
                    for competition in competitions:
                        fixture = _fixture(
                            match_type=match_type,
                            gender=gender,
                            international=international,
                            date=day,
                            competition=competition,
                        )
                        for country in countries:
                            for rank_on_day, matches_on_day, rank_at_venue in ranks:
                                yield sessions.infer(fixture, country, rank_on_day, matches_on_day, rank_at_venue)


def test_pre_play_window_ends_before_any_rule_can_place_a_start() -> None:
    """DATA-05: the same-day windows end one hour before the earliest hour any rule table
    holds, and no window infer() can produce starts before that hour -- so the windows
    cannot contain an hour of play under any rule."""
    assert sessions.EARLIEST_START_HOUR == 9
    assert features.PRE_PLAY_END_HOUR == 8
    assert min(window.start_hour for window in _every_inferable_window()) >= sessions.EARLIEST_START_HOUR
    assert min(window.start_hour for window in _every_inferable_window()) > features.PRE_PLAY_END_HOUR


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
