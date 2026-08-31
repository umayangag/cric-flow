package config

import "strings"

// Win model families selection can maximise and the scorecard can display.
const (
	// WinModelWindowedForm is the original model over windowed player-form aggregates
	// (ml.win_features). Held-out AUC 0.56-0.63 once its leak was removed (S-3c).
	WinModelWindowedForm = "windowed_form"
	// WinModelXI is the XI-responsive rating model (ml.xi). Every input is a function of
	// the two elevens; held-out AUC 0.72-0.75 in limited-overs formats (S-10).
	WinModelXI = "xi"
)

// SelectionUsesXIWinModel reports whether selection.win_model asks for the XI-responsive
// model. Anything other than "xi" (including unset) keeps the windowed-form model, so an
// old config.json keeps its old behaviour.
func SelectionUsesXIWinModel(cfg *Config) bool {
	return cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.Selection.WinModel), WinModelXI)
}
