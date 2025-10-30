batting_dataset_query = "SELECT  runs, batting_data.balls, batting_data.fours, batting_data.sixes, batting_position, " \
                        "player.batting_consistency, player_form_data.batting_form, weather.temp,weather.wind, weather.rain, " \
                        "weather.humidity, weather.cloud, weather.pressure, weather.viscosity, match_details.inning, " \
                        "" \
                        "match_details.batting_session, match_details.toss, player_venue_data.batting_venue, " \
                        "player_opposition_data.batting_opposition, season.id as season_id,player.player_name FROM `batting_data` left join player on " \
                        "player_id=player.id left join (select * from weather_data WHERE SESSION like 'batting') " \
                        "as weather on batting_data.match_id=weather.match_id left join match_details " \
                        "on match_details.match_id=weather.match_id left join venue " \
                        "on venue.id= match_details.venue_id " \
                        "left join opposition on opposition.id=match_details.opposition_id " \
                        "left join season on season.id=match_details.season_id " \
                        "left join player_venue_data on batting_data.player_id=player_venue_data.player_id " \
                        "and match_details.venue_id=player_venue_data.venue_id " \
                        "left join player_opposition_data on batting_data.player_id=player_opposition_data.player_id " \
                        "and match_details.opposition_id=player_opposition_data.opposition_id " \
                        "left join player_form_data on batting_data.player_id=player_form_data.player_id " \
                        "and match_details.season_id=player_form_data.season_id"

batting_win_dataset_query = "SELECT  match_details.score, match_details.balls, runs, batting_data.balls, batting_data.fours, batting_data.sixes, strike_rate, batting_data.match_id, batting_position," \
                            "player.player_name, player.batting_consistency, player_form_data.batting_form, weather.temp,weather.wind, weather.rain, " \
                            "weather.humidity, weather.cloud, weather.pressure, weather.viscosity, match_details.inning, " \
                            "" \
                            "match_details.batting_session, match_details.toss, player_venue_data.batting_venue, " \
                            "player_opposition_data.batting_opposition, season.id as season_id, match_details.result FROM `batting_data` left join player on " \
                            "player_id=player.id left join (select * from weather_data WHERE SESSION like 'batting') " \
                            "as weather on batting_data.match_id=weather.match_id left join match_details " \
                            "on match_details.match_id=weather.match_id left join venue " \
                            "on venue.id= match_details.venue_id " \
                            "left join opposition on opposition.id=match_details.opposition_id " \
                            "left join season on season.id=match_details.season_id " \
                            "left join player_venue_data on batting_data.player_id=player_venue_data.player_id " \
                            "and match_details.venue_id=player_venue_data.venue_id " \
                            "left join player_opposition_data on batting_data.player_id=player_opposition_data.player_id " \
                            "and match_details.opposition_id=player_opposition_data.opposition_id " \
                            "left join player_form_data on batting_data.player_id=player_form_data.player_id " \
                            "and match_details.season_id=player_form_data.season_id"

bowling_dataset_query = "SELECT  bowling_data.runs, bowling_data.balls, bowling_data.wickets,  " \
                        "player.bowling_consistency, player_form_data.bowling_form, weather.temp,weather.wind, weather.rain, " \
                        "weather.humidity, weather.cloud, weather.pressure, weather.viscosity, match_details.inning, " \
                        "" \
                        "match_details.bowling_session, match_details.toss, player_venue_data.bowling_venue, " \
                        "player_opposition_data.bowling_opposition, season.id as season_id, player.player_name FROM `bowling_data` left join player on " \
                        "player_id=player.id left join (select * from weather_data WHERE SESSION like 'bowling') " \
                        "as weather on bowling_data.match_id=weather.match_id left join match_details " \
                        "on match_details.match_id=weather.match_id left join venue " \
                        "on venue.id= match_details.venue_id " \
                        "left join opposition on opposition.id=match_details.opposition_id " \
                        "left join season on season.id=match_details.season_id " \
                        "left join player_venue_data on bowling_data.player_id=player_venue_data.player_id " \
                        "and match_details.venue_id=player_venue_data.venue_id " \
                        "left join player_opposition_data on bowling_data.player_id=player_opposition_data.player_id " \
                        "and match_details.opposition_id=player_opposition_data.opposition_id " \
                        "left join player_form_data on bowling_data.player_id=player_form_data.player_id " \
                        "and match_details.season_id=player_form_data.season_id"