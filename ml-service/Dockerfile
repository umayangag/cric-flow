# syntax=docker/dockerfile:1
# Build with context from repo root: docker-compose uses context: . and dockerfile: ml-service/Dockerfile
#
#   docker build -f ml-service/Dockerfile -t cric-app-ml:latest .
#
# One image. There were two stages -- `serve` and a heavier `train` carrying pycaret,
# autogluon and shap -- until P-6 deleted the auto-tune stack they existed for. What is
# left of hyperparameter search is a three-point grid inside the retrain step, which runs
# on scikit-learn, so the serving image is also the training image.

FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1
WORKDIR /app
# Build tools required by some Python packages (e.g., numpy uses Meson);
# libpq-dev for the PostgreSQL client libraries psycopg2-binary links against.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       build-essential \
       pkg-config \
       ninja-build \
       libpq-dev \
    && rm -rf /var/lib/apt/lists/*

COPY ml-service/requirements.txt .
RUN pip install --upgrade pip \
    && pip install --no-cache-dir -r ./requirements.txt
COPY ml-service/config.default.json .
COPY ml-service/config.json .
COPY ml-service/app ./app
COPY ml-service/ml ./ml
# Which commit these sources are. Nothing in the image can work it out: there is no
# version-control binary and no repository directory, so `git rev-parse` fails and every
# run built through /admin/train/* recorded an empty commit (EVAL-12). The build knows,
# so the build tells it. Last, after the COPYs, so changing it rebuilds nothing above.
# docker-compose passes it through from the Makefile; a bare `docker build` should pass
# `--build-arg GIT_SHA=$(git rev-parse HEAD)`. Left unset, the manifest records `unknown`.
ARG GIT_SHA=""
ENV GIT_SHA=${GIT_SHA}
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
