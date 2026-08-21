"""Feature contract constants shared with go-app."""


# -------------------- Feature input models --------------------

# Raw windowed stat names (v2 contract); used for validation and backtest build.
BATTING_RAW_STAT_KEYS = [
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
BOWLING_RAW_STAT_KEYS = [
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
