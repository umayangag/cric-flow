package cricsheet

import (
	"context"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/domain"
)

// Loader defines an abstraction to enumerate and load raw Cricsheet documents
// (YAML/JSON) from a source such as a directory or archive. It should return
// the raw bytes and a logical identifier (e.g., filename) for diagnostic/logging.
//go:generate mockery --name Loader --output internal/mocks --case underscore
// NOTE: mocks generated to go-app/internal/mocks (run from module root)
type Loader interface {
	// List returns logical identifiers of available scorecard documents.
	List(ctx context.Context, inDir string) ([]string, error)
	// Load returns the raw contents for the specified identifier.
	Load(ctx context.Context, inDir string, id string) ([]byte, error)
}

// Parser defines an abstraction that converts a raw Cricsheet document into
// one or more domain entities suitable for persistence.
//go:generate mockery --name Parser --output internal/mocks --case underscore
type Parser interface {
	// Parse decodes a raw document and returns zero or more matches.
	Parse(ctx context.Context, raw []byte) ([]domain.Match, error)
}
