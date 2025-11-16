package exportqueries

import "context"

// seqFlagKey is an unexported context key type to avoid collisions.
type seqFlagKey struct{}

var keySeqFlag seqFlagKey

// WithSeqEnabled stores whether sequence feature columns are enabled in the context.
func WithSeqEnabled(ctx context.Context, enabled bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, keySeqFlag, enabled)
}

// IsSeqEnabled reads the sequence feature flag from context.
// Returns false when not present.
func IsSeqEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v := ctx.Value(keySeqFlag)
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
