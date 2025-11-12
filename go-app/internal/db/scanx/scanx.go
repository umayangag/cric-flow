package scanx

// RowScanner is the minimal subset used to iterate and scan rows.
// It intentionally mirrors the methods we rely on from pgx.Rows / db.Rows,
// without importing those packages to avoid cyclic dependencies.
type RowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close()
}

// CountReturningOnes consumes rows produced by queries like
// "... RETURNING 1" and returns the number of affected rows.
// It closes the rows before returning.
func CountReturningOnes(rows RowScanner) (int64, error) {
	if rows == nil {
		return 0, nil
	}
	defer rows.Close()
	var affected int64
	for rows.Next() {
		var one int
		if err := rows.Scan(&one); err != nil {
			return 0, err
		}
		affected++
	}
	return affected, nil
}
