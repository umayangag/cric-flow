"""Pre-match weather context (X-2, docs/EXTERNAL_DATA_PLAN.md).

The archive says where a match was played and on what day, never what the air was like.
This package acquires that from the Open-Meteo ERA5 archive -- a venue -> coordinates table
(``geocoding``), a session window inferred per match because Cricsheet carries no start
times (``sessions``), one reduced day of readings per (venue, date) cached as a real answer
(``archive``) -- and turns them into the four feature families X-2 gates (``features``).
Everything a feature reads is fixed before the first ball (H-21): readings in the hours
before the inferred start, and daily values of the days before it.

The reduced observations and the coordinates are tracked in git (``reference-data/``): they
are CC BY 4.0 and re-acquiring them is a rate-limited pass over ~20,000 venue-days.
"""
