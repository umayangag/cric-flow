import os

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
from dataset_definitions import *
from imblearn.over_sampling import SMOTE
from sklearn import metrics, preprocessing, svm
from sklearn.ensemble import RandomForestClassifier, RandomForestRegressor
from sklearn.linear_model import LinearRegression
from sklearn.metrics import confusion_matrix
from sklearn.model_selection import cross_val_score, train_test_split
from sklearn.multioutput import MultiOutputRegressor
from sklearn.naive_bayes import GaussianNB
from sklearn.neural_network import MLPClassifier

RF = RandomForestClassifier(n_estimators=100)
gnb = GaussianNB()
clf = MLPClassifier(
    solver="lbfgs", alpha=1e-5, hidden_layer_sizes=(10, 5), random_state=1, max_iter=5000
)
SVM = svm.SVC(kernel="linear", C=1)
regr = RandomForestRegressor(max_depth=100, random_state=0)
reg = LinearRegression()
mltreg = MultiOutputRegressor(regr)
predictor = mltreg

dirname = os.path.dirname(__file__)
dataset_source = os.path.join(dirname, "output\\bowling_encoded.csv")

input_data = pd.read_csv(dataset_source)
training_input_columns = input_bowling_columns.copy()
training_input_columns.remove("player_name")

X = input_data[training_input_columns]
y = input_data[output_bowling_columns]  # Labels

input_scaler = preprocessing.StandardScaler().fit(X)
input_data_scaled = input_scaler.transform(X)
X = pd.DataFrame(data=input_data_scaled, columns=X.columns)

output_scaler = preprocessing.StandardScaler().fit(y)
output_data_scaled = output_scaler.transform(y)
y = pd.DataFrame(data=output_data_scaled, columns=y.columns)

# X_train, X_test, y_train, y_test = train_test_split(X, y, test_size=0.1)
train_set = 1465
X_train = X.iloc[:train_set, :]
X_test = X.iloc[train_set + 1 :, :]
y_train = y.iloc[:train_set]
y_test = y.iloc[train_set + 1 :]
predictor.fit(X_train, y_train)

predictor.fit(X_train, y_train)


def calculate_econ(row):
    if row["deliveries"] == 0:
        return 0
    return row["runs_conceded"] * 6 / row["deliveries"]


def predict_bowling(dataset):
    scaled_dataset = input_scaler.transform(dataset)
    predicted = predictor.predict(scaled_dataset)
    result = pd.DataFrame(
        output_scaler.inverse_transform(predicted), columns=output_bowling_columns
    )
    for column in y.columns:
        dataset[column] = result[column]
    dataset["econ"] = dataset.apply(lambda row: calculate_econ(row), axis=1)
    return dataset


def bowling_predict_test():
    y_pred = predictor.predict(X_test)

    comparison = {}
    comparison["actual"] = y_test.to_numpy()
    comparison["predicted"] = y_pred

    for i in range(0, len(y_pred)):
        print(
            comparison["actual"][i],
            " ",
            comparison["predicted"][i],
            "",
        )

    # print("Error:", metrics.mean_absolute_error(y_test, y_pred))
    #
    # print('Mean Absolute Error:', metrics.mean_absolute_error(y_test, y_pred))
    # print('Mean Squared Error:', metrics.mean_squared_error(y_test, y_pred))
    # print('Root Mean Squared Error:', np.sqrt(metrics.mean_squared_error(y_test, y_pred)))
    # print('R2:', metrics.r2_score(y_test, y_pred))

    print("Cross Validation Score:", cross_val_score(predictor, X, y, cv=10).max())


if __name__ == "__main__":
    bowling_predict_test()
