# Cricsheet import

## Why “imported” count can be less than files on disk

The importer reports the number of **files successfully imported**. If that number is lower than the number of `.json` files in the directory, it is usually because:

1. **Fail-fast (default)**  
   The importer runs with `-fail-fast=true` (default). On the **first** file that causes an error (parse error, unsupported `match_type`, DB error, etc.), the run **stops**. The reported count is how many files were imported **before** that failure. Remaining files are never processed.

2. **Only top-level files**  
   Only **direct** children of the input directory are considered; subdirectories are not recursed. So `.json` files inside subdirs are never listed or imported.

3. **Supported match types**  
   Only these `info.match_type` values are supported: `TEST`, `MDM`, `ODI`, `ODM`, `T20`, `T20I`, `IT20`. Any other value (e.g. `Friendly`, `T10`) causes an error and, with fail-fast, stops the run.

## How to find and fix the failing file(s)

- **See which file failed:**  
  Re-run the import and check the logs. The first error will log the failing filename and the error (e.g. `unsupported match_type`, parse error).

- **Import the rest and list skipped files:**  
  Run with fail-fast disabled so one bad file does not stop the whole run:

  From repo root:

  ```bash
  make cricsheet-import FAIL_FAST=0
  ```

  Or run the CLI directly: `cd go-app && go run ./cmd/cricsheet-importer -in=../data/go-app/cricsheet -fail-fast=false`

  The importer will skip failing files, log each with `"import failed, skipping"`, and at the end log a summary with `skipped_files`. You can then fix or remove those files and re-run.

## Scope

- **Input:** Directory given by `-in` / `GO_APP_INPUT_DIR` (e.g. `data/go-app/cricsheet`).
- **Listing:** `os.ReadDir(dir)` — only top-level entries; entries with `IsDir()==true` are skipped; only names ending in `.json` (case-insensitive) are processed.
- **Count:** One file = one match; the returned count is the number of files that completed `ImportMatchFile` without error.

## References

- Importer: `go-app/cmd/cricsheet-importer`, `go-app/internal/cricsheet/ingest.go`
- Format detection: `go-app/internal/cricsheet/format.go` (`DetectFormat`)
- Config: `docs/CONFIG.md` (`cricsheet_dir`)
