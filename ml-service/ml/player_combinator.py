def calculate_overall_performance(input_df, match_id):
    team_df = input_df.copy()
    magic_number = 11 / len(team_df)  # this is to compensate players missing from actual 11
    extras = 14.26
    total_score = team_df["runs_scored"].sum() * magic_number + extras
    target = team_df["runs_conceded"].sum() * magic_number
    total_balls_faced = team_df["balls_faced"].sum() * magic_number
    total_wickets_taken = team_df["wickets_taken"].sum() * magic_number

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
