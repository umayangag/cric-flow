"""XI-responsive win model.

Everything the win model consumes is a function of the eleven names on each side, computed
as-of the match date by a single chronological pass over ball-by-ball history (see
``ml.xi.ratings``). It replaced the windowed-form aggregates of the old win model as the
objective for team selection: see docs/WIN_PROB_SELECTION_PR_CHECKLIST.md, S-9 and S-10.
"""
