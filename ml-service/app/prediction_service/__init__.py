"""Prediction orchestration for the windowed-form win model.

``app.prediction_service.endpoints`` is what is left of it: the batting, bowling, extras
and innings handlers, the reconciled generate-match projection and the server-side team
optimiser all went with their models in P-5. Prediction proper lives in ``app.xi_service``.
"""
