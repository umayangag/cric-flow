package db

import "context"

//go:generate mockery --name=Connector --output=internal/db/mocks --filename=mock_connector.go --outpkg=mocks --with-expecter

// Connector abstracts DB connectivity used by command packages for orchestration tests.
// RealConnector uses the existing package-level Connect.
type Connector interface {
	Connect(ctx context.Context) error
}

type RealConnector struct{}

func (RealConnector) Connect(ctx context.Context) error {
	_, err := Connect(ctx)
	return err
}
