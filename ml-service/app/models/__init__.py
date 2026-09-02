"""Pydantic models for the ML service HTTP API.

``app.models.predict`` carries the windowed-form win model's request and response shapes;
``app.models.xi`` carries the XI layer's -- selection, win probability, the performance model
and the simulator. Import the submodules directly; the transitional re-exports that lived here
were removed in C6-2.
"""
