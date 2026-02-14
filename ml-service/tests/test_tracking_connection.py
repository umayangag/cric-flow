import os
import sys
import unittest
from unittest.mock import MagicMock, patch

# Add ml-service root to path
ml_service_path = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
sys.path.insert(0, ml_service_path)

# Mock psycopg2 and dotenv
mock_psycopg2_module = MagicMock()
sys.modules["psycopg2"] = mock_psycopg2_module
sys.modules["dotenv"] = MagicMock()

# Import modules under test
try:
    from ml import db, tracking
except ImportError as e:
    print(f"Import failed: {e}")
    db = None
    tracking = None


class TestDBConnection(unittest.TestCase):
    def setUp(self):
        # Reset the mock before each test
        mock_psycopg2_module.reset_mock()

    def test_db_connection_no_defaults(self):
        """Verify that db.get_db_connection does NOT use insecure defaults."""
        if db is None:
            self.fail("Could not import ml.db")

        with patch.dict(os.environ, {}, clear=True):
            db.get_db_connection()

            # db.py imports psycopg2 at top level, so it uses the mock we injected
            # We can check calls on the injected mock
            call_kwargs = mock_psycopg2_module.connect.call_args[1]

            # Check for hardcoded defaults
            self.assertIsNone(call_kwargs.get("user"), f"Expected None, got {call_kwargs.get('user')}")
            self.assertIsNone(call_kwargs.get("password"), f"Expected None, got {call_kwargs.get('password')}")


if __name__ == "__main__":
    unittest.main()
