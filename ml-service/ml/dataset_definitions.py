import numpy as np

player_columns = [
    "id",
    "player_name",
    "is_wicket_keeper",
    "is_retired",
    "batting_consistency",
    "bowling_consistency",
]

input_batting_columns = [
    "batting_consistency",
    "batting_form",
    "batting_temp",
    "batting_wind",
    "batting_rain",
    "batting_humidity",
    "batting_cloud",
    "batting_pressure",
    "batting_viscosity",
    "batting_inning",
    "batting_session",
    "toss",
    "venue",
    "opposition",
    "season",
    "player_name",
]

output_batting_columns = ["runs_scored", "balls_faced", "fours_scored", "sixes_scored", "batting_position"]
derived_batting_columns = ["batting_contribution", "strike_rate"]
input_bowling_columns = [
    "bowling_consistency",
    "bowling_form",
    "bowling_temp",
    "bowling_wind",
    "bowling_rain",
    "bowling_humidity",
    "bowling_cloud",
    "bowling_pressure",
    "bowling_viscosity",
    "batting_inning",
    "bowling_session",
    "toss",
    "bowling_venue",
    "bowling_opposition",
    "season",
    "player_name",
]
output_bowling_columns = ["runs_conceded", "deliveries", "wickets_taken"]
derived_bowling_columns = ["bowling_contribution", "econ"]
match_summary_columns = ["total_score", "total_wickets", "total_balls", "target", "extras", "match_number", "result"]

all_batting_columns = np.concatenate((input_batting_columns, output_batting_columns, derived_batting_columns))
all_bowling_columns = np.concatenate((input_bowling_columns, output_bowling_columns, derived_bowling_columns))
