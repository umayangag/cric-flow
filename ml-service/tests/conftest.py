# Ensure the ml-service package root is on sys.path so that `import app.*` works
# regardless of where pytest is invoked from.
import sys
from pathlib import Path

# This file lives in <repo>/ml-service/tests/conftest.py
# We want to add <repo>/ml-service to sys.path
ML_SERVICE_ROOT = Path(__file__).resolve().parents[1]
if str(ML_SERVICE_ROOT) not in sys.path:
    sys.path.insert(0, str(ML_SERVICE_ROOT))
