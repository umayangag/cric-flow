import json
import os


def load_config():
    # Resolve config path: prefer ML_SERVICE_CONFIG env var; else try ./config.json
    cfg_path = os.environ.get("ML_SERVICE_CONFIG") or os.path.join(os.getcwd(), "config.json")
    try:
        with open(cfg_path, "r", encoding="utf-8") as f:
            return json.load(f)
    except Exception:
        return {}


config = load_config()


def calculate_overall_performance(input_df, match_id):
    team_df = input_df.copy()
    team_size = config["team_prediction"]["team_size"]
    default_extras = config["team_prediction"]["default_extras"]

    magic_number = team_size / len(team_df)  # this is to compensate players missing from actual 11
    extras = default_extras
    total_score = team_df["runs_scored"].sum() * magic_number + extras
    target = team_df["runs_conceded"].sum() * magic_number
    total_balls_faced = team_df["balls_faced"].sum() * magic_number

    team_df["total_score"] = total_score * magic_number
    team_df["total_wickets"] = 10
    team_df["total_balls"] = total_balls_faced
    team_df["target"] = target
    team_df["extras"] = extras
    team_df["match_number"] = match_id

    def calculate_batting_contribution(row, key):
        return row[key] / total_score

    def calculate_bowling_contribution(row, key):
        return row[key] / target

    team_df["bowling_contribution"] = team_df.apply(
        lambda row: calculate_bowling_contribution(row, "runs_conceded"), axis=1
    )
    team_df["batting_contribution"] = team_df.apply(
        lambda row: calculate_batting_contribution(row, "runs_scored"), axis=1
    )

    return team_df
