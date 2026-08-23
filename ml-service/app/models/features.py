"""Player-level feature input models for /predict endpoints."""

from typing import Optional

from pydantic import BaseModel, Field, field_validator


class BattingFeatures(BaseModel):
    # Raw windowed stats (v2; from feature_raw_stats_snapshots)
    batting_mean_w3: float = Field(default=0.0, ge=0)
    batting_mean_w5: float = Field(default=0.0, ge=0)
    batting_mean_w10: float = Field(default=0.0, ge=0)
    batting_mean_w20: float = Field(default=0.0, ge=0)
    batting_std_w5: float = Field(default=0.0, ge=0)
    batting_std_w10: float = Field(default=0.0, ge=0)
    batting_max_w10: float = Field(default=0.0, ge=0)
    batting_min_w10: float = Field(default=0.0, ge=0)
    batting_median_w10: float = Field(default=0.0, ge=0)
    batting_last_1: float = Field(default=0.0, ge=0)
    batting_last_2: float = Field(default=0.0, ge=0)
    batting_last_3: float = Field(default=0.0, ge=0)
    batting_career_mean: float = Field(default=0.0, ge=0)
    batting_career_count: float = Field(default=0.0, ge=0)
    batting_pct_zero_w10: float = Field(default=0.0, ge=0)
    batting_trend_w5: float = Field(default=0.0, ge=0)
    batting_days_since_last: float = Field(default=0.0, ge=0)
    batting_innings_in_last_90d: float = Field(default=0.0, ge=0)
    batting_temp: int
    batting_wind: int = Field(..., ge=0)
    batting_rain: int = Field(..., ge=0)
    batting_humidity: int = Field(..., ge=0)
    batting_cloud: int = Field(..., ge=0)
    batting_pressure: int = Field(..., ge=0)
    batting_viscosity: int = Field(..., ge=0, le=1)
    batting_inning: int = Field(..., ge=1, le=2)
    batting_session: int = Field(..., ge=1, le=3)
    toss: int = Field(..., ge=0, le=1)
    venue: float
    opposition: float
    match_month_sin: float = Field(default=0.0)
    match_month_cos: float = Field(default=0.0)
    match_day_of_week_sin: float = Field(default=0.0)
    match_day_of_week_cos: float = Field(default=0.0)
    player_name: str
    format: Optional[str] = None
    # Optional sequential features (0 when absent; used when go-app exports with -enable-seq)
    bat_prev_sr: float = Field(default=0.0, ge=0)
    bat_prev_out_rate: float = Field(default=0.0, ge=0)
    bat_window_sr_12_pp: float = Field(default=0.0, ge=0)
    bat_window_boundary_rate_12_pp: float = Field(default=0.0, ge=0)
    bat_entry_sr_1_6: float = Field(default=0.0, ge=0)
    bat_set_sr_13_30: float = Field(default=0.0, ge=0)
    bat_react_after_dot_sr: float = Field(default=0.0, ge=0)
    bat_after_k_dots_boundary_p_k2: float = Field(default=0.0, ge=0)

    @field_validator("format", mode="before")
    def _format_upper(cls, v: Optional[str]) -> Optional[str]:
        if v is None:
            return v
        v2 = v.strip().upper()
        # Allow empty/unknown formats by returning normalized value
        return v2


class BowlingFeatures(BaseModel):
    # Raw windowed stats (v2; from feature_raw_stats_snapshots)
    bowling_mean_w3: float = Field(default=0.0, ge=0)
    bowling_mean_w5: float = Field(default=0.0, ge=0)
    bowling_mean_w10: float = Field(default=0.0, ge=0)
    bowling_mean_w20: float = Field(default=0.0, ge=0)
    bowling_std_w5: float = Field(default=0.0, ge=0)
    bowling_std_w10: float = Field(default=0.0, ge=0)
    bowling_max_w10: float = Field(default=0.0, ge=0)
    bowling_min_w10: float = Field(default=0.0, ge=0)
    bowling_median_w10: float = Field(default=0.0, ge=0)
    bowling_last_1: float = Field(default=0.0, ge=0)
    bowling_last_2: float = Field(default=0.0, ge=0)
    bowling_last_3: float = Field(default=0.0, ge=0)
    bowling_career_mean: float = Field(default=0.0, ge=0)
    bowling_career_count: float = Field(default=0.0, ge=0)
    bowling_pct_zero_w10: float = Field(default=0.0, ge=0)
    bowling_trend_w5: float = Field(default=0.0, ge=0)
    bowling_days_since_last: float = Field(default=0.0, ge=0)
    bowling_innings_in_last_90d: float = Field(default=0.0, ge=0)
    bowling_temp: int
    bowling_wind: int = Field(..., ge=0)
    bowling_rain: int = Field(..., ge=0)
    bowling_humidity: int = Field(..., ge=0)
    bowling_cloud: int = Field(..., ge=0)
    bowling_pressure: int = Field(..., ge=0)
    bowling_viscosity: int = Field(..., ge=0, le=1)
    batting_inning: int = Field(..., ge=1, le=2)
    bowling_session: int = Field(..., ge=1, le=3)
    toss: int = Field(..., ge=0, le=1)
    bowling_venue: float
    bowling_opposition: float
    match_month_sin: float = Field(default=0.0)
    match_month_cos: float = Field(default=0.0)
    match_day_of_week_sin: float = Field(default=0.0)
    match_day_of_week_cos: float = Field(default=0.0)
    player_name: str
    format: Optional[str] = None
    # Optional sequential features (0 when absent; used when go-app exports with -enable-seq)
    bowl_prev_wkt_rate: float = Field(default=0.0, ge=0)
    bowl_window_econ_24_death: float = Field(default=0.0, ge=0)
    bowl_window_wkt_rate_24_death: float = Field(default=0.0, ge=0)
    bowl_extras_wide_rate_pp: float = Field(default=0.0, ge=0)
    bowl_react_after_boundary_wkt_rate_next: float = Field(default=0.0, ge=0)
    bowl_spell_first_over_wkt_rate: float = Field(default=0.0, ge=0)
    bowl_over_ball1_wkt_rate: float = Field(default=0.0, ge=0)
    bowl_over_ball6_wkt_rate: float = Field(default=0.0, ge=0)

    @field_validator("format", mode="before")
    def _format_upper(cls, v: Optional[str]) -> Optional[str]:
        if v is None:
            return v
        v2 = v.strip().upper()
        return v2
