package fake

import "context"

// Connector is a no-op deterministic fake implementing the db.Connector contract.
// It always succeeds, allowing offline orchestration tests.
type Connector struct{}

func (Connector) Connect(_ context.Context) error { return nil }
