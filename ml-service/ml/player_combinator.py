from create_final_dataset import get_actual_players_who_played


def actual_team_players(pool_df, match_id):
    actual_player_df, wicket_keepers, bowlers = get_actual_players_who_played(match_id)
    return pool_df[pool_df["player_name"].isin(actual_player_df["player_name"].to_numpy())]


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

    print(magic_number, team_df["runs_scored"].sum(), team_df["balls_faced"].sum(), extras)
    print(
        magic_number,
        team_df["runs_conceded"].sum(),
        team_df["deliveries"].sum(),
        team_df["wickets_taken"].sum(),
    )

    if team_df["balls_faced"].sum() > 300:
        print(
            "Total Score:",
            (team_df["runs_scored"].sum() * 300 / team_df["balls_faced"].sum()) + extras,
        )
    else:
        print("Total Score:", (team_df["runs_scored"].sum() * magic_number) + extras)

    if team_df["deliveries"].sum() > 300:
        print("Runs given:", team_df["runs_conceded"].sum() * 300 / team_df["deliveries"].sum())
    elif team_df["wickets_taken"].sum() > 10:
        print("Runs given:", team_df["runs_conceded"].sum() * 10 / team_df["wickets_taken"].sum())
    else:
        print("Runs given:", team_df["runs_conceded"].sum())

    # TODO: for evaluation
    # SELECT * FROM `match_details` WHERE `wickets` < 10 AND `balls` < 300 AND `target` IS NOT NULL
    # where the team stopped batting, since they have chased the opposition target before the 50 overs
    # need to compare predicted score with score for 50 overs. because the predicted score will always be high

    return team_df
