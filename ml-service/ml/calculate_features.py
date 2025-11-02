import pandas as pd
from db import get_db_connection


def calculate_features(db_connection):
    db_cursor = db_connection.cursor()
    db_cursor.execute("SELECT id, player_name FROM player")
    players_list = db_cursor.fetchall()
    db_cursor.execute("SELECT id, season_name FROM season")
    season_list = db_cursor.fetchall()
    db_cursor.execute("SELECT id, code FROM match_format")
    format_list = db_cursor.fetchall()
    db_cursor.execute("SELECT id, venue_name FROM venue")
    venue_list = db_cursor.fetchall()
    db_cursor.execute("SELECT id, opposition_name FROM opposition")
    opposition_list = db_cursor.fetchall()

    for season in season_list:
        for format in format_list:
            for player in players_list:
                # Batting form
                db_cursor.execute(
                    "SELECT bd.description, bd.runs, bd.balls, bd.minutes, bd.fours, bd.sixes, bd.strike_rate, md.season_id, md.format_id FROM batting_data bd LEFT JOIN match_details md ON bd.match_id = md.match_id WHERE bd.player_id = %s AND md.season_id = %s AND md.format_id = %s",
                    (player[0], season[0], format[0]),
                )
                player_data = db_cursor.fetchall()
                inning_count = len(player_data)
                if inning_count > 5:
                    df = pd.DataFrame(player_data)
                    not_out_count = len(df[df[0] == "not out"])
                    sum_of_runs = df[1].sum()
                    average_strike_rate = df[6].mean()
                    if inning_count != not_out_count:
                        average_score = sum_of_runs / (inning_count - not_out_count)
                    else:
                        average_score = sum_of_runs

                    centuries = len(df[df[1] >= 100])
                    fifties = len(df[df[1] >= 50]) - centuries
                    zeros = len(df[df[1] == 0])

                    form = (
                        0.4262 * average_score
                        + 0.2566 * inning_count
                        + 0.1510 * average_strike_rate
                        + 0.0787 * centuries
                        + 0.0556 * fifties
                        - 0.0328 * zeros
                    )
                    db_cursor.execute(
                        "INSERT INTO player_form_data_fmt (player_id, season_id, format_id, batting_form, bowling_form) VALUES (%s, %s, %s, %s, 0) ON CONFLICT (player_id, season_id, format_id) DO UPDATE SET batting_form = %s",
                        (player[0], season[0], format[0], form, form),
                    )

                # Bowling form
                db_cursor.execute(
                    "SELECT bw.balls, bw.runs, bw.wickets, bw.econ, md.season_id, md.format_id FROM bowling_data bw LEFT JOIN match_details md ON bw.match_id = md.match_id WHERE bw.player_id = %s AND md.season_id = %s AND md.format_id = %s",
                    (player[0], season[0], format[0]),
                )
                player_data = db_cursor.fetchall()
                inning_count = len(player_data)
                if inning_count > 5:
                    df = pd.DataFrame(player_data)
                    total_overs = df[0].sum() / 6
                    strike_rate = df[0].sum() / df[2].sum()
                    average = df[1].sum() / df[2].sum()
                    ff = len(df[df[2] >= 5])

                    form = (
                        0.4174 * total_overs
                        + 0.2634 * inning_count
                        + 0.1602 * strike_rate
                        + 0.0975 * average
                        + 0.0615 * ff
                    )
                    db_cursor.execute(
                        "UPDATE player_form_data_fmt SET bowling_form = %s WHERE player_id = %s AND season_id = %s AND format_id = %s",
                        (form, player[0], season[0], format[0]),
                    )

    for venue in venue_list:
        for format in format_list:
            for player in players_list:
                # Batting venue
                db_cursor.execute(
                    "SELECT bd.description, bd.runs, bd.balls, bd.minutes, bd.fours, bd.sixes, bd.strike_rate, md.venue_id, md.format_id FROM batting_data bd LEFT JOIN match_details md ON bd.match_id = md.match_id WHERE bd.player_id = %s AND md.venue_id = %s AND md.format_id = %s",
                    (player[0], venue[0], format[0]),
                )
                player_data = db_cursor.fetchall()
                inning_count = len(player_data)
                if inning_count > 5:
                    df = pd.DataFrame(player_data)
                    not_out_count = len(df[df[0] == "not out"])
                    sum_of_runs = df[1].sum()
                    average_strike_rate = df[6].mean()
                    if inning_count != not_out_count:
                        average_score = sum_of_runs / (inning_count - not_out_count)
                    else:
                        average_score = sum_of_runs

                    centuries = len(df[df[1] >= 100])
                    fifties = len(df[df[1] >= 50]) - centuries
                    highest_score = df[1].max()

                    venue_performance = (
                        0.4262 * average_score
                        + 0.2566 * inning_count
                        + 0.1510 * average_strike_rate
                        + 0.0787 * centuries
                        + 0.0556 * fifties
                        + 0.0328 * highest_score
                    )
                    db_cursor.execute(
                        "INSERT INTO player_venue_data_fmt (player_id, venue_id, format_id, batting_venue, bowling_venue) VALUES (%s, %s, %s, %s, 0) ON CONFLICT (player_id, venue_id, format_id) DO UPDATE SET batting_venue = %s",
                        (player[0], venue[0], format[0], venue_performance, venue_performance),
                    )

                # Bowling venue
                db_cursor.execute(
                    "SELECT bw.balls, bw.runs, bw.wickets, bw.econ, md.venue_id, md.format_id FROM bowling_data bw LEFT JOIN match_details md ON bw.match_id = md.match_id WHERE bw.player_id = %s AND md.venue_id = %s AND md.format_id = %s",
                    (player[0], venue[0], format[0]),
                )
                player_data = db_cursor.fetchall()
                inning_count = len(player_data)
                if inning_count > 5:
                    df = pd.DataFrame(player_data)
                    total_overs = df[0].sum() / 6
                    strike_rate = df[0].sum() / df[2].sum()
                    average = df[1].sum() / df[2].sum()
                    ff = len(df[df[2] >= 5])

                    venue_performance = (
                        0.4174 * total_overs
                        + 0.2634 * inning_count
                        + 0.1602 * strike_rate
                        + 0.0975 * average
                        + 0.0615 * ff
                    )
                    db_cursor.execute(
                        "UPDATE player_venue_data_fmt SET bowling_venue = %s WHERE player_id = %s AND venue_id = %s AND format_id = %s",
                        (venue_performance, player[0], venue[0], format[0]),
                    )

    for opposition in opposition_list:
        for format in format_list:
            for player in players_list:
                # Batting opposition
                db_cursor.execute(
                    "SELECT bd.description, bd.runs, bd.balls, bd.minutes, bd.fours, bd.sixes, bd.strike_rate, md.opposition_id, md.format_id FROM batting_data bd LEFT JOIN match_details md ON bd.match_id = md.match_id WHERE bd.player_id = %s AND md.opposition_id = %s AND md.format_id = %s",
                    (player[0], opposition[0], format[0]),
                )
                player_data = db_cursor.fetchall()
                inning_count = len(player_data)
                if inning_count > 5:
                    df = pd.DataFrame(player_data)
                    not_out_count = len(df[df[0] == "not out"])
                    sum_of_runs = df[1].sum()
                    average_strike_rate = df[6].mean()
                    if inning_count != not_out_count:
                        average_score = sum_of_runs / (inning_count - not_out_count)
                    else:
                        average_score = sum_of_runs

                    centuries = len(df[df[1] >= 100])
                    fifties = len(df[df[1] >= 50]) - centuries
                    highest_score = df[1].max()

                    opposition_performance = (
                        0.4262 * average_score
                        + 0.2566 * inning_count
                        + 0.1510 * average_strike_rate
                        + 0.0787 * centuries
                        + 0.0556 * fifties
                        + 0.0328 * highest_score
                    )
                    db_cursor.execute(
                        "INSERT INTO player_opposition_data_fmt (player_id, opposition_id, format_id, batting_opposition, bowling_opposition) VALUES (%s, %s, %s, %s, 0) ON CONFLICT (player_id, opposition_id, format_id) DO UPDATE SET batting_opposition = %s",
                        (
                            player[0],
                            opposition[0],
                            format[0],
                            opposition_performance,
                            opposition_performance,
                        ),
                    )

                # Bowling opposition
                db_cursor.execute(
                    "SELECT bw.balls, bw.runs, bw.wickets, bw.econ, md.opposition_id, md.format_id FROM bowling_data bw LEFT JOIN match_details md ON bw.match_id = md.match_id WHERE bw.player_id = %s AND md.opposition_id = %s AND md.format_id = %s",
                    (player[0], opposition[0], format[0]),
                )
                player_data = db_cursor.fetchall()
                inning_count = len(player_data)
                if inning_count > 5:
                    df = pd.DataFrame(player_data)
                    total_overs = df[0].sum() / 6
                    strike_rate = df[0].sum() / df[2].sum()
                    average = df[1].sum() / df[2].sum()
                    ff = len(df[df[2] >= 5])

                    opposition_performance = (
                        0.4174 * total_overs
                        + 0.2634 * inning_count
                        + 0.1602 * strike_rate
                        + 0.0975 * average
                        + 0.0615 * ff
                    )
                    db_cursor.execute(
                        "UPDATE player_opposition_data_fmt SET bowling_opposition = %s WHERE player_id = %s AND opposition_id = %s AND format_id = %s",
                        (opposition_performance, player[0], opposition[0], format[0]),
                    )

    for season in season_list:
        for format in format_list:
            for player in players_list:
                # Batting consistency
                db_cursor.execute(
                    "SELECT description, runs, balls, minutes, fours, sixes, strike_rate FROM batting_data where player_id=%s",
                    (player[0],),
                )
                player_data = db_cursor.fetchall()
                inning_count = len(player_data)
                if inning_count > 10:
                    df = pd.DataFrame(player_data)
                    not_out_count = len(df[df[0] == "not out"])
                    sum_of_runs = df[1].sum()
                    average_strike_rate = df[6].mean()
                    average_score = sum_of_runs / (inning_count - not_out_count)

                    centuries = len(df[df[1] >= 100])
                    fifties = len(df[df[1] >= 50]) - centuries
                    zeros = len(df[df[1] == 0])

                    consistency = (
                        0.4262 * average_score
                        + 0.2566 * inning_count
                        + 0.1510 * average_strike_rate
                        + 0.0787 * centuries
                        + 0.0556 * fifties
                        - 0.0328 * zeros
                    )
                    db_cursor.execute(
                        "INSERT INTO player_consistency_data_fmt (player_id, season_id, format_id, batting_consistency, bowling_consistency) VALUES (%s, %s, %s, %s, 0) ON CONFLICT (player_id, season_id, format_id) DO UPDATE SET batting_consistency = %s",
                        (player[0], season[0], format[0], consistency, consistency),
                    )

                # Bowling consistency
                db_cursor.execute(
                    "SELECT balls, runs, wickets, econ FROM bowling_data where player_id=%s",
                    (player[0],),
                )
                player_data = db_cursor.fetchall()
                inning_count = len(player_data)
                if inning_count > 10:
                    df = pd.DataFrame(player_data)
                    total_overs = df[0].sum() / 6
                    strike_rate = df[0].sum() / df[2].sum()
                    average = df[1].sum() / df[2].sum()
                    ff = len(df[df[2] >= 5])

                    consistency = (
                        0.4174 * total_overs
                        + 0.2634 * inning_count
                        + 0.1602 * strike_rate
                        + 0.0975 * average
                        + 0.0615 * ff
                    )
                    db_cursor.execute(
                        "UPDATE player_consistency_data_fmt SET bowling_consistency = %s WHERE player_id = %s AND season_id = %s AND format_id = %s",
                        (consistency, player[0], season[0], format[0]),
                    )

    db_connection.commit()


if __name__ == "__main__":
    db_connection = get_db_connection()
    calculate_features(db_connection)
