package phase

import "testing"

type tcase struct {
	name          string
	formatCode    string
	formatID      int
	ballSeq       int
	inningsLength int
	want          string
}

func TestPhaseForCode_T20(t *testing.T) {
	cases := []tcase{
		{"t20_pp_boundary", "T20", 0, 36, 120, PhasePowerplay},
		{"t20_middle_start", "T20", 0, 37, 120, PhaseMiddle},
		{"t20_death_start", "T20", 0, 91, 120, PhaseDeath},
		{"t20_short_innings_no_death", "T20", 0, 80, 80, PhaseMiddle},
		{"t20i_equivalent", "T20I", 0, 20, 120, PhasePowerplay},
	}
	for _, tc := range cases {
		if got := PhaseForCode(tc.formatCode, tc.ballSeq, tc.inningsLength); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestPhaseForCode_ODI(t *testing.T) {
	cases := []tcase{
		{"odi_pp1_boundary", "ODI", 0, 60, 300, PhasePowerplay},
		{"odi_middle_start", "ODI", 0, 61, 300, PhaseMiddle},
		{"odi_death_start", "ODI", 0, 241, 300, PhaseDeath},
		{"odi_short_innings_no_death", "ODI", 0, 200, 200, PhaseMiddle},
	}
	for _, tc := range cases {
		if got := PhaseForCode(tc.formatCode, tc.ballSeq, tc.inningsLength); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestPhaseForCode_Test(t *testing.T) {
	cases := []tcase{
		{"test_any_ball_all", "TEST", 0, 1, 0, PhaseAll},
		{"test_any_ball_known_len", "TEST", 0, 250, 540, PhaseAll},
	}
	for _, tc := range cases {
		if got := PhaseForCode(tc.formatCode, tc.ballSeq, tc.inningsLength); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestPhaseFor_IDMapping(t *testing.T) {
	cases := []tcase{
		{"id_t20", "", 3, 10, 120, PhasePowerplay},
		{"id_t20i", "", 4, 95, 120, PhaseDeath},
		{"id_odi", "", 2, 70, 300, PhaseMiddle},
		{"id_test", "", 1, 300, 0, PhaseAll},
		{"id_unknown", "", 99, 10, 60, PhaseAll},
	}
	for _, tc := range cases {
		if got := PhaseFor(tc.formatID, tc.ballSeq, tc.inningsLength); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}
