package teamselect

// Options holds CLI flags for team-select.
type Options struct {
	MatchID       int64
	Format        string
	Season        string
	PoolPath      string
	TeamSize      int
	MinBowlers    int
	RequireKeeper bool
	FromDB        bool
}
