from db import get_db_connection
import pandas as pd


def calculate_features(db_connection):
    db_cursor = db_connection.cursor()
    db_cursor.execute("SELECT id, player_name FROM player")
    players_list = db_cursor.fetchall()
    db_cursor.execute("SELECT id, season_name FROM season")
    season_list = db_cursor.fetchall()
    db_cursor.execute("SELECT id, code FROM match_format")
    format_list = db_cursor.fetchall()

    for season in season_list:
        for format in format_list:
            for player in players_list:
                # Batting form
                db_cursor.execute(
                    f'SELECT bd.description, bd.runs, bd.balls, bd.minutes, bd.fours, bd.sixes, bd.strike_rate, md.season_id, md.format_id FROM batting_data bd LEFT JOIN match_details md ON bd.match_id = md.match_id WHERE bd.player_id = {player[0]} AND md.season_id = {season[0]} AND md.format_id = {format[0]}'
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

                    form = 0.4262 * average_score + 0.2566 * inning_count + 0.1510 * average_strike_rate + 0.0787 * centuries + 0.0556 * fifties - 0.0328 * zeros
                    print(f'Batting form for {player[1]} in season {season[1]} and format {format[1]}: {form}')
                    db_cursor.execute(f'INSERT INTO player_form_data_fmt (player_id, season_id, format_id, batting_form, bowling_form) VALUES ({player[0]}, {season[0]}, {format[0]}, {form}, 0) ON CONFLICT (player_id, season_id, format_id) DO UPDATE SET batting_form = {form}')

                # Bowling form
                db_cursor.execute(
                    f'SELECT bw.balls, bw.runs, bw.wickets, bw.econ, md.season_id, md.format_id FROM bowling_data bw LEFT JOIN match_details md ON bw.match_id = md.match_id WHERE bw.player_id = {player[0]} AND md.season_id = {season[0]} AND md.format_id = {format[0]}'
                )
                player_data = db_cursor.fetchall()
                inning_count = len(player_data)
                if inning_count > 5:
                    df = pd.DataFrame(player_data)
                    total_overs = df[0].sum() / 6
                    strike_rate = df[0].sum() / df[2].sum()
                    average = df[1].sum() / df[2].sum()
                    ff = len(df[df[2] >= 5])

                    form = 0.4174 * total_overs + 0.2634 * inning_count + 0.1602 * strike_rate + 0.0975 * average + 0.0615 * ff
                    print(f'Bowling form for {player[1]} in season {season[1]} and format {format[1]}: {form}')
                    db_cursor.execute(f'UPDATE player_form_data_fmt SET bowling_form = {form} WHERE player_id = {player[0]} AND season_id = {season[0]} AND format_id = {format[0]}')

    # Batting consistency
    for player in players_list:
        db_cursor.execute(
            f'SELECT description, runs, balls, minutes, fours, sixes, strike_rate FROM batting_data where player_id={player[0]}')
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

            consistency = 0.4262 * average_score + 0.2566 * inning_count + 0.1510 * average_strike_rate + 0.0787 * centuries + 0.0556 * fifties - 0.0328 * zeros
            print(f'Batting consistency for {player[1]}: {consistency}')
            db_cursor.execute(f'INSERT INTO player_consistency_data_fmt (player_id, season_id, format_id, batting_consistency, bowling_consistency) VALUES ({player[0]}, {season[0]}, {format[0]}, {consistency}, 0) ON CONFLICT (player_id, season_id, format_id) DO UPDATE SET batting_consistency = {consistency}')

    # Bowling consistency
    for player in players_list:
        db_cursor.execute(
            f'SELECT balls, runs, wickets, econ FROM bowling_data where player_id={player[0]}')
        player_data = db_cursor.fetchall()
        inning_count = len(player_data)
        if inning_count > 10:
            df = pd.DataFrame(player_data)
            total_overs = df[0].sum() / 6
            strike_rate = df[0].sum() / df[2].sum()
            average = df[1].sum() / df[2].sum()
            ff = len(df[df[2] >= 5])

            consistency = 0.4174 * total_overs + 0.2634 * inning_count + 0.1602 * strike_rate + 0.0975 * average + 0.0615 * ff
            print(f'Bowling consistency for {player[1]}: {consistency}')
            db_cursor.execute(f'UPDATE player_consistency_data_fmt SET bowling_consistency = {consistency} WHERE player_id = {player[0]} AND season_id = {season[0]} AND format_id = {format[0]}')

    db_connection.commit()


if __name__ == "__main__":
    db_connection = get_db_connection()
    calculate_features(db_connection)
