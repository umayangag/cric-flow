# Legacy Python Code (src/)

This directory contains legacy Python code from an earlier implementation. The canonical pipeline is:

- **Data import**: `go-app` Cricsheet importer (`make cricsheet-import`)
- **Precompute**: `go-app` precompute commands
- **ML training**: `ml-service` Python (under `ml-service/`)
- **Team selection**: `go-app` + `ml-service` API

The scripts here (scrapers, createdb, team_selection, preprocessing) are **not** part of the active pipeline. They are kept for reference and potential migration of logic. New development should target `go-app/` and `ml-service/`.
