"""XI-responsive win model.

Everything the win model consumes is a function of the eleven names on each side, computed
as-of the match date by a single chronological pass over ball-by-ball history (see
``ml.xi.ratings``). It replaced the windowed-form aggregates of the old win model as the
objective for team selection: see docs/ML_PIPELINE_REARCHITECTURE_PLAN.md, 3 and 8.
"""
