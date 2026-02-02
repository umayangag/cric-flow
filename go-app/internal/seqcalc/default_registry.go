package seqcalc

// NewDefaultRegistry builds and returns the default registry of sequence calculators
// used by commands. It registers no-op calculators first to ensure all targets are
// present, then overlays concrete implementations where available.
func NewDefaultRegistry() *Registry {
	return NewRegistry(append(
		NewNoopCalculators(),
		NewBatTransitionsCalculator(),
		NewBowlSequencesCalculator(),
		NewPlayerWindowsCalculator(),
		NewReactionCalculator(),
		NewDotStreaksCalculator(),
		NewDisciplineCalculator(),
		NewWicketModesCalculator(),
		NewSpellsCalculator(),
		NewOverPosCalculator(),
		NewEndPressureCalculator(),
	)...)
}
