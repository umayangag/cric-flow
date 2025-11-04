import os

import pandas as pd
from sklearn import preprocessing
from sklearn.neural_network import MLPClassifier

clf = MLPClassifier(
    solver="sgd",
    activation="tanh",
    alpha=1e-5,
    hidden_layer_sizes=(43, 11, 1),
    random_state=1,
    max_iter=10000,
)
predictor = clf

dirname = os.path.dirname(__file__)
# Support both Windows-style and POSIX paths, prefer POSIX
_candidates = [
    os.path.join(dirname, "..", "team_selection", "final_dataset.csv"),
    os.path.join(dirname, "..\\team_selection\\final_dataset.csv"),
]

dataset_source = next((p for p in _candidates if os.path.exists(p)), None)
X_cols = None


class _DummyPredictor:
    def predict_proba(self, X):
        import numpy as _np

        n = getattr(X, "shape", [len(X) if hasattr(X, "__len__") else 1])[0]
        return _np.tile(_np.array([[0.5, 0.5]]), (n, 1))


# Train win predictor if dataset present; otherwise use a dummy baseline to keep the service runnable.
if dataset_source is not None:
    input_data = pd.read_csv(dataset_source)
    X = input_data[
        [
            "batting_consistency",
            "batting_form",
            "batting_temp",
            "batting_wind",
            "batting_rain",
            "batting_humidity",
            "batting_cloud",
            "batting_pressure",
            "batting_viscosity",
            "batting_inning",
            "batting_session",
            "toss",
            "venue",
            "opposition",
            # 'season',
            "runs_scored",
            "balls_faced",
            "fours_scored",
            "sixes_scored",
            "batting_position",
            "batting_contribution",
            "strike_rate",
            "total_score",
            "total_wickets",
            "total_balls",
            "target",
            "extras",
            # 'match_number',
            "bowling_consistency",
            "bowling_form",
            "bowling_temp",
            "bowling_wind",
            "bowling_rain",
            "bowling_humidity",
            "bowling_cloud",
            "bowling_pressure",
            "bowling_viscosity",
            "bowling_session",
            "bowling_venue",
            "bowling_opposition",
            "runs_conceded",
            "deliveries",
            "wickets_taken",
            "bowling_contribution",
            "econ",
        ]
    ]
    y = input_data["result"]  # Labels

    scaler = preprocessing.StandardScaler().fit(X)
    data_scaled = scaler.transform(X)
    X = pd.DataFrame(data=data_scaled, columns=X.columns)
    X_cols = list(X.columns)

    predictor.fit(X, y)
else:
    scaler = None
    X_cols = None
    predictor = _DummyPredictor()


def predict_for_team(team_data):
    print(set(X.columns) - set(team_data.columns))
    team_performance = team_data.copy()
    predicted = predictor.predict_proba(team_performance)
    df = pd.DataFrame(predicted, columns=["lose", "win"])
    team_performance["winning_probability"] = df["win"].to_numpy()
    return team_performance, df["win"].mean()
