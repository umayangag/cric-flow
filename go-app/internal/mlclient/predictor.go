package mlclient

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

//go:generate mockery --name=Predictor --output=internal/mlclient/mocks --filename=mock_predictor.go --outpkg=mocks --with-expecter

// Predictor defines the minimal interface required by command packages
// to obtain winning predictions. The concrete Service implements this.
type Predictor interface {
	PredictWin(ctx context.Context, players []predictor.PlayerPrediction) ([]predictor.PlayerPrediction, error)
}
