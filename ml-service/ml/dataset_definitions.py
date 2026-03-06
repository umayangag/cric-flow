import numpy as np

# v2: raw windowed stat names from configs/feature_vectors.json (single source of truth).
try:
    from app.feature_config import get_raw_stat_feature_names

    _batting_raw_stat_columns = get_raw_stat_feature_names("batting")
    _bowling_raw_stat_columns = get_raw_stat_feature_names("bowling")
except (ImportError, RuntimeError):  # noqa: S110 (config may be missing in some test envs)
    _batting_raw_stat_columns = [
        "batting_mean_w3",
        "batting_mean_w5",
        "batting_mean_w10",
        "batting_mean_w20",
        "batting_std_w5",
        "batting_std_w10",
        "batting_max_w10",
        "batting_min_w10",
        "batting_median_w10",
        "batting_last_1",
        "batting_last_2",
        "batting_last_3",
        "batting_career_mean",
        "batting_career_count",
        "batting_pct_zero_w10",
        "batting_trend_w5",
        "batting_days_since_last",
        "batting_innings_in_last_90d",
    ]
    _bowling_raw_stat_columns = [
        "bowling_mean_w3",
        "bowling_mean_w5",
        "bowling_mean_w10",
        "bowling_mean_w20",
        "bowling_std_w5",
        "bowling_std_w10",
        "bowling_max_w10",
        "bowling_min_w10",
        "bowling_median_w10",
        "bowling_last_1",
        "bowling_last_2",
        "bowling_last_3",
        "bowling_career_mean",
        "bowling_career_count",
        "bowling_pct_zero_w10",
        "bowling_trend_w5",
        "bowling_days_since_last",
        "bowling_innings_in_last_90d",
    ]

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
    *_batting_raw_stat_columns,  # v2
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
    # Fielding aggregates appended (must match go-app exporters' order)
    "catches",
    "run_outs",
    "stumpings",
    "runouts_direct_hits",
    "fielding_involvements",
]

output_batting_columns = [
    "runs_scored",
    "balls_faced",
    "fours_scored",
    "sixes_scored",
    "batting_position",
]
derived_batting_columns = ["batting_contribution", "strike_rate"]
input_bowling_columns = [
    "bowling_consistency",
    "bowling_form",
    *_bowling_raw_stat_columns,  # v2
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
    # Fielding aggregates appended (must match go-app exporters' order)
    "catches",
    "run_outs",
    "stumpings",
    "runouts_direct_hits",
    "fielding_involvements",
]
output_bowling_columns = ["runs_conceded", "deliveries", "wickets_taken"]
derived_bowling_columns = ["bowling_contribution", "econ"]
match_summary_columns = [
    "total_score",
    "total_wickets",
    "total_balls",
    "target",
    "extras",
    "match_number",
    "result",
]

all_batting_columns = np.concatenate((input_batting_columns, output_batting_columns, derived_batting_columns))
all_bowling_columns = np.concatenate((input_bowling_columns, output_bowling_columns, derived_bowling_columns))
