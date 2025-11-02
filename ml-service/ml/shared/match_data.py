from config.mysql import get_db_connection

db_connection = get_db_connection()
db_cursor = db_connection.cursor()


def get_match_data(match_id, session):
    db_cursor.execute(
        f"SELECT inning, {session}_session, toss, venue_id, opposition_id, season_id, score, wickets, balls, target, extras, match_number, result FROM match_details where match_id={match_id}"
    )
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
    db_cursor.execute(
        f'SELECT temp, wind, rain, humidity, cloud, pressure, viscosity FROM weather_data where match_id={match_id} and session="{session}"'
    )
    temp, wind, rain, humidity, cloud, pressure, viscosity = db_cursor.fetchall()[0]
    return temp, wind, rain, humidity, cloud, pressure, viscosity


def get_player_metric(match_id, type, player, metric, metric_type, metric_id):
    if metric_id is not None:
        if metric == "form":
            db_cursor.execute(
                f"SELECT {type}_form FROM player_form_data where player_id={player[0]} and season_id={metric_id}"
            )
        elif metric == "venue":
            db_cursor.execute(
                f"SELECT {type}_venue FROM player_venue_data where player_id={player[0]} and venue_id={metric_id}"
            )
        elif metric == "opposition":
            db_cursor.execute(
                f"SELECT {type}_opposition FROM player_opposition_data where player_id={player[0]} and opposition_id={metric_id}"
            )

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


def actual_team_players(player_pool, match_id):
    db_cursor.execute(
        f"SELECT player.id, player.player_name, player.is_wicket_keeper, player.is_retired, player.batting_consistency, player.bowling_consistency FROM player inner join batting_data on player.id=batting_data.player_id where batting_data.match_id={match_id}"
    )
    team_players = db_cursor.fetchall()
    team_players_df = pd.DataFrame(team_players, columns=player_columns)
    player_pool_df = player_pool[player_pool["player_name"].isin(team_players_df["player_name"])]
    return player_pool_df
