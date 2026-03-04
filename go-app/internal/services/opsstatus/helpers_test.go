package opsstatus

// assertErr is a sentinel error for test failures.
type assertErr struct{}

func (assertErr) Error() string { return "test error" }
