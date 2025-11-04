from ml.db import get_db_connection

db_connection = get_db_connection()
db_cursor = db_connection.cursor()


def get_match_data(match_id, session):
    db_cursor.execute((f"SELECT inning, {session}_session, toss, venue_id, opposition_id, season_id, " f"score, wickets, balls, target, extras, match_number, result " f"FROM match_details where match_id={match_id}"))
    (
        inning,
        session,
        toss,
        venue_id,
        opposition_id,
        season_id,
        score,
        wickets,
        balls,
        target,
        extras,
        match_number,
        result,
    ) = db_cursor.fetchall()[0]
    return (
        inning,
        session,
        toss,
        venue_id,
        opposition_id,
        season_id,
        score,
        wickets,
        balls,
        target,
        extras,
        match_number,
        result,
    )


def get_weather_data(match_id, session):
    db_cursor.execute((f"SELECT temp, wind, rain, humidity, cloud, pressure, viscosity FROM weather_data " f'where match_id={match_id} and session="{session}"'))
    temp, wind, rain, humidity, cloud, pressure, viscosity = db_cursor.fetchall()[0]
    return temp, wind, rain, humidity, cloud, pressure, viscosity


def get_player_metric(match_id, type, player, metric, metric_type, metric_id):
    if metric_id is not None:
        if metric == "form":
            db_cursor.execute((f"SELECT {type}_form FROM player_form_data_fmt where player_id={player[0]} " f"and season_id={metric_id} and format_id={player[1]}"))
        elif metric == "venue":
            db_cursor.execute((f"SELECT {type}_venue FROM player_venue_data_fmt " f"where player_id={player[0]} and venue_id={metric_id} " f"and format_id={player[1]}"))
        elif metric == "opposition":
            db_cursor.execute((f"SELECT {type}_opposition FROM player_opposition_data_fmt where player_id={player[0]} " f"and opposition_id={metric_id} and format_id={player[1]}"))

        metric_val = db_cursor.fetchall()
        if len(metric_val) > 0:
            return metric_val[0][0]
    return 0


def encode_viscosity(viscosity):
    if viscosity == "dry":
        return 0
    elif viscosity == "humid":
        return 1
    elif viscosity == "windy":
        return 2
    return 0


def encode_session(session):
    if "morning" in session:
        return 0
    elif "afternoon" in session:
        return 1
    elif "evening" in session:
        return 2
    return 0
