def _get_team_prediction_config():
    """Load team_prediction from config (ml.config or fallback)."""
    try:
        from ml.config import _load

        cfg = _load()
        tp = cfg.get("team_prediction") or {}
        return {
            "team_size": int(tp.get("team_size", 11)),
            "max_wickets_per_innings": int(tp.get("max_wickets_per_innings", 10)),
        }
    except Exception:
        return {"team_size": 11, "max_wickets_per_innings": 10}


def calculate_overall_performance(input_df, match_id, predicted_extras=0.0):
    """Extras must be supplied from a model or historical average (e.g. format/venue average); no default constant."""
    cfg = _get_team_prediction_config()
    team_df = input_df.copy()
    team_size = cfg["team_size"]
    max_wickets = cfg["max_wickets_per_innings"]

    magic_number = team_size / len(team_df)  # this is to compensate players missing from actual 11
    extras = float(predicted_extras)
    total_score = team_df["runs_scored"].sum() * magic_number + extras
    target = team_df["runs_conceded"].sum() * magic_number
    total_balls_faced = team_df["balls_faced"].sum() * magic_number

    team_df.loc[:, "total_score"] = total_score * magic_number
    team_df.loc[:, "total_wickets"] = max_wickets
    team_df.loc[:, "total_balls"] = total_balls_faced
    team_df.loc[:, "target"] = target
    team_df.loc[:, "extras"] = extras
    team_df.loc[:, "match_number"] = match_id

    def calculate_batting_contribution(row, key):
        return row[key] / total_score

    def calculate_bowling_contribution(row, key):
        return row[key] / target

    team_df.loc[:, "bowling_contribution"] = team_df.apply(
        lambda row: calculate_bowling_contribution(row, "runs_conceded"), axis=1
    )
    team_df.loc[:, "batting_contribution"] = team_df.apply(
        lambda row: calculate_batting_contribution(row, "runs_scored"), axis=1
    )

    return team_df
