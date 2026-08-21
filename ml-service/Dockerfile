# syntax=docker/dockerfile:1
# Build with context from repo root: docker-compose uses context: . and dockerfile: ml-service/Dockerfile
FROM python:3.12-slim AS base
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1
WORKDIR /app
# Install build tools required by some Python packages (e.g., numpy uses Meson)
# Also install libpq-dev for PostgreSQL client libraries needed by psycopg2-binary
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       build-essential \
       pkg-config \
       ninja-build \
       libpq-dev \
    && rm -rf /var/lib/apt/lists/*
COPY ml-service/requirements.txt .
RUN pip install --upgrade pip \
    && pip install --no-cache-dir \
       torch torchvision torchaudio \
       --index-url https://download.pytorch.org/whl/cpu \
    && pip install --no-cache-dir -r ./requirements.txt
COPY ml-service/config.default.json .
COPY ml-service/config.json .
COPY ml-service/app ./app
COPY ml-service/ml ./ml
# Feature names for training/prediction (app/feature_config.py); override with FEATURE_CONFIG_PATH
COPY configs/feature_vectors.json /app/configs/feature_vectors.json
ENV FEATURE_CONFIG_PATH=/app/configs/feature_vectors.json
EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
