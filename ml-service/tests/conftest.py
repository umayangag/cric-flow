# Ensure the ml-service package root is on sys.path so that `import app.*` works
# regardless of where pytest is invoked from.
import os
import sys
import tempfile
from pathlib import Path

# This file lives in <repo>/ml-service/tests/conftest.py
# We want to add <repo>/ml-service to sys.path
ML_SERVICE_ROOT = Path(__file__).resolve().parents[1]
if str(ML_SERVICE_ROOT) not in sys.path:
    sys.path.insert(0, str(ML_SERVICE_ROOT))

# Prevent tests from loading real trained artifacts (heavy joblib models).
# Set ML_SERVICE_OUTPUT_DIR to an empty temp dir before any test imports app.main.
# This runs at conftest load time, which occurs before test collection/import.
if "ML_SERVICE_OUTPUT_DIR" not in os.environ and "MODELS_DIR" not in os.environ:
    os.environ["ML_SERVICE_OUTPUT_DIR"] = tempfile.mkdtemp(prefix="ml_service_test_")
