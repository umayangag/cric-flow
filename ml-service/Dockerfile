# syntax=docker/dockerfile:1
# Build with context from repo root: docker-compose uses context: . and dockerfile: ml-service/Dockerfile
#
# Two targets:
#   serve  (default)  requirements-serve.txt -- no pycaret/autogluon/shap, so no torch,
#                     tensorboard or chronos either. This is what docker-compose runs.
#   train             requirements.txt -- the full set, for PyCaret/AutoGluon-assisted
#                     auto-tune and SHAP explanations.
#
# The serve image still runs /admin/train/* and /admin/train/auto-tune: those shell out to
# `python -m ml.train_*` and `ml.auto_tune`, which need only scikit-learn and Optuna. PyCaret,
# AutoGluon and SHAP each sit behind a guarded import with a graceful fallback, so auto-tune
# degrades to Optuna-only rather than failing. Build the train target when you want their
# model ranking.
#
#   docker build -f ml-service/Dockerfile --target serve -t cric-app-ml:serve .
#   docker build -f ml-service/Dockerfile --target train -t cric-app-ml:train .

FROM python:3.12-slim AS base
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

# ── serve ────────────────────────────────────────────────────────────────────
FROM base AS serve
COPY ml-service/requirements-serve.txt .
RUN pip install --upgrade pip \
    && pip install --no-cache-dir -r ./requirements-serve.txt
COPY ml-service/config.default.json .
COPY ml-service/config.json .
COPY ml-service/app ./app
COPY ml-service/ml ./ml
# Feature names for training/prediction (app/feature_config.py); override with FEATURE_CONFIG_PATH
COPY configs/feature_vectors.json /app/configs/feature_vectors.json
ENV FEATURE_CONFIG_PATH=/app/configs/feature_vectors.json
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]

# ── train ────────────────────────────────────────────────────────────────────
FROM base AS train
COPY ml-service/requirements.txt .
# CPU-only torch: the default wheels pull CUDA and are several GB larger.
RUN pip install --upgrade pip \
    && pip install --no-cache-dir \
       torch torchvision torchaudio \
       --index-url https://download.pytorch.org/whl/cpu \
    && pip install --no-cache-dir -r ./requirements.txt
COPY ml-service/config.default.json .
COPY ml-service/config.json .
COPY ml-service/app ./app
COPY ml-service/ml ./ml
COPY configs/feature_vectors.json /app/configs/feature_vectors.json
ENV FEATURE_CONFIG_PATH=/app/configs/feature_vectors.json
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
