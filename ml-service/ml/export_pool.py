import sys

import pandas as pd
from batting_regressor import predict_batting
from bowling_regressor import predict_bowling
from dataset_definitions import (
    input_batting_columns,
    input_bowling_columns,
    player_columns,
)
from db import get_db_connection
from fill_missing_attributes import fill_missing_attributes
from match_data import (
    encode_session,
    encode_viscosity,
    get_match_data,
    get_player_metric,
    get_weather_data,
)


def get_bowling_performance(player_list, match_id):
    (
        inning,
        session,
        toss,
        venue_id,
        opposition_id,
        season_id,
        score,
        total_wickets,
        balls,
        target,
        extras,
        match_number,
        result,
    ) = get_match_data(match_id, "bowling")
    temp, wind, rain, humidity, cloud, pressure, viscosity = get_weather_data(match_id, "bowling")

    data_array = []

    for player in player_list.iterrows():
        player_obj = player[1]
        _ = player_obj[0]
        player_name = player_obj[1]
        player_form = get_player_metric(match_id, "bowling", player_obj, "form", "season", season_id - 1)
        player_venue = get_player_metric(match_id, "bowling", player_obj, "venue", "venue", venue_id)
        player_opposition = get_player_metric(
            match_id, "bowling", player_obj, "opposition", "opposition", opposition_id
        )
        data_array.append(
            [
                player[1]["bowling_consistency"],
                player_form,
                temp,
                wind,
                rain,
                humidity,
                cloud,
                pressure,
                encode_viscosity(viscosity),
                inning,
                encode_session(session),
                toss,
                player_venue,
                player_opposition,
                season_id,
                player_name,
            ]
        )
    dataset = pd.DataFrame(data_array, columns=input_bowling_columns)
    predicted = predict_bowling(dataset.loc[:, dataset.columns != "player_name"])
    predicted["player_name"] = dataset["player_name"]
    return predicted


def get_batting_performance(player_list, match_id):
    (
        inning,
        session,
        toss,
        venue_id,
        opposition_id,
        season_id,
        score,
        wickets,
        all_balls,
        target,
        extras,
        match_number,
        result,
    ) = get_match_data(match_id, "batting")
    temp, wind, rain, humidity, cloud, pressure, viscosity = get_weather_data(match_id, "batting")

    data_array = []

    for player in player_list.iterrows():
        player_obj = player[1]
        _ = player_obj[0]
        player_name = player_obj[1]
        player_form = get_player_metric(match_id, "batting", player_obj, "form", "season", season_id - 1)
        player_venue = get_player_metric(match_id, "batting", player_obj, "venue", "venue", venue_id)
        player_opposition = get_player_metric(
            match_id, "batting", player_obj, "opposition", "opposition", opposition_id
        )

        data_array.append(
            [
                player[1]["batting_consistency"],
                player_form,
                temp,
                wind,
                rain,
                humidity,
                cloud,
                pressure,
                encode_viscosity(viscosity),
                inning,
                encode_session(session),
                toss,
                player_venue,
                player_opposition,
                season_id,
                player_name,
            ]
        )
    dataset = pd.DataFrame(data_array, columns=input_batting_columns)
    predicted = predict_batting(dataset.loc[:, dataset.columns != "player_name"])
    predicted["player_name"] = dataset["player_name"]
    return predicted


def get_player_pool():
    db_connection = get_db_connection()
    db_cursor = db_connection.cursor()
    db_cursor.execute(
        "SELECT * FROM player WHERE is_retired=0 and (batting_consistency != 0 or bowling_consistency != 0)"
    )
    player_list = db_cursor.fetchall()
    player_df = pd.DataFrame(player_list, columns=player_columns)
    return player_df


if __name__ == "__main__":
    if len(sys.argv) > 1:
        match_id = int(sys.argv[1])
    else:
        match_id = 1193505

    players = get_player_pool()

    batting_df = get_batting_performance(players, match_id)
    bowling_df = get_bowling_performance(players, match_id)

    bowling_df = bowling_df.loc[:, bowling_df.columns != "toss"]
    bowling_df = bowling_df.loc[:, bowling_df.columns != "season"]
    bowling_df = bowling_df.loc[:, bowling_df.columns != "batting_inning"]
    player_pool = pd.merge(batting_df, bowling_df, on="player_name", how="left")
    player_pool = fill_missing_attributes(player_pool)
    player_pool.to_csv("pool.csv", index=False)
