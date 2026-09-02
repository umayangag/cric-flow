# Bug backlog

Bugs found while working an item of `docs/FOLLOW_UP_PLAN.md` that are outside that item's
scope. A non-blocking bug is recorded here and left alone; a blocking one gets its own
`fix/<slug>` PR stacked under the item, and the row names it.

| id | found during | symptom | where in the code | why it matters | blocking | status |
|---|---|---|---|---|---|---|
| B-1 | A-1 | Reading team-level context for a team, venue or head-to-head pair the state has never seen *writes* that key into the state: `team_context` indexes `defaultdict`s, so a serving request naming an unknown opposition id (or one that names teams but no venue) grows `team_elo`, `team_results`, `head_to_head`, `venue_bat_first` and `team_venue_matches` by one entry per read. The numbers served are the right neutral ones; the loaded state is not the state that was loaded. | `ml-service/ml/xi/ratings.py`, `RatingState.team_context` (and `update`, whose `defaultdict` indexing is correct there) | The D-7 defect class (plan §10.6): a read that mutates the serving state. `_read_slots` closed it for players in P-5; the keyed tables were left as they were. `fixture_context` (A-1) reads with `.get` for this reason. Harmless to the numbers today; it is what makes a long-lived `XiStore` drift under traffic and would make an H-8 comparison after serving depend on what was served. | no | open |
