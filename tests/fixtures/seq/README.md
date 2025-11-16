Sequence feature fixtures (deterministic, offline)

Structure
- t20/innings_basic.json: One short innings with a mix of dots, singles, boundaries, wides/no‑balls, a wicket, and over transitions.
  - Intent: Validate ETL `ball_event` emission, ball_seq monotonic on legal balls, illegal ball handling (shares prior legal index), wicket capture, phase boundaries around PP/middle.
  - Quick checks:
    - `.deliveries | length` <= 20
    - Contains at least one `extras_kind` in ["wide","no_ball"].
    - Contains a `wicket_kind`.

- t20/overs_order.json: Six overs emphasizing bowler over order and alternation patterns.
  - Intent: Validate bowling sequence A→B detection over consecutive overs for the same fielding side.
  - Quick checks:
    - `.overs | length` == 6
    - Over order yields pairs: 201→202, 202→201, 201→203, 203→202, 202→203

- t20/dot_streaks.json: Engineered sequences for k‑dot streak logic and immediate reaction after boundary/wicket.
  - Intent: Validate reaction features (next‑ball after event) and dot‑streak behavior for k∈{0,1,2,3}.
  - Quick checks:
    - `.sequences | length` >= 4
    - Contains a wicket after k=3 dots and a boundary reaction.

Usage
- Tests should load JSON directly without DB/network. Keep IDs small deterministic integers.
- These fixtures are not Cricsheet raw; they are simplified intended for unit tests around sequence logic.

Notes
- `ball_seq` counts only legal deliveries; illegal balls have `is_legal=false` and repeat the prior legal `ball_seq` for ordering context. Use `(over, ball)` to recover exact chronological order.
