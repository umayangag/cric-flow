import importlib
import os
from pathlib import Path

from fastapi.testclient import TestClient


def test_health_lists_artifacts_and_metadata_with_stats(tmp_path):
    # Prepare a fake models dir with artifact and metadata files
    models_dir = Path(tmp_path)
    # Create zero-byte joblib files to exercise os.stat path; loader errors are swallowed
    (models_dir / "batting_model_ODI.joblib").write_bytes(b"")
    (models_dir / "bowling_model_T20.joblib").write_bytes(b"")
    # Create metadata json files
    (models_dir / "batting_metadata_ODI.json").write_text("{}", encoding="utf-8")
    (models_dir / "bowling_metadata_T20.json").write_text("{}", encoding="utf-8")

    os.environ["ML_SERVICE_OUTPUT_DIR"] = str(models_dir)

    app_module = importlib.import_module("app.main")
    importlib.reload(app_module)

    client = TestClient(app_module.app)
    resp = client.get("/health")
    assert resp.status_code == 200
    data = resp.json()

    # Verify artifacts include our files and have stat fields
    bat_files = {a["file"] for a in data["artifacts"]["batting"]}
    bowl_files = {a["file"] for a in data["artifacts"]["bowling"]}
    assert "batting_model_ODI.joblib" in bat_files
    assert "bowling_model_T20.joblib" in bowl_files

    # Find the specific entries to check presence of size_bytes and modified
    def has_stat(entry):
        return "size_bytes" in entry and "modified" in entry

    assert any(e.get("file") == "batting_model_ODI.joblib" and has_stat(e) for e in data["artifacts"]["batting"])
    assert any(e.get("file") == "bowling_model_T20.joblib" and has_stat(e) for e in data["artifacts"]["bowling"])

    # Verify metadata listing picks up our json files
    assert "batting_metadata_ODI.json" in data["metadata"]["batting"]
    assert "bowling_metadata_T20.json" in data["metadata"]["bowling"]
