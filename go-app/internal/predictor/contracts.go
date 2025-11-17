package predictor

import "context"

// Predictor defines the minimal interface required by command packages
// to obtain winning predictions. The concrete Service implements this.
type Predictor interface {
	PredictWin(ctx context.Context, players []PlayerPrediction) ([]PlayerPrediction, error)
}
