-- 0002_datasets.sql
--
-- The dataset registry (ops plan A-3): one row per acquired Cricsheet dataset.
--
-- data_migrations already records every *run*, but a run is not a dataset. Two
-- fetches of an unchanged archive are two runs and one dataset; a fetch followed by
-- an extract is two runs and still one dataset. Asking "which data produced this
-- model?" against a table of runs means reconstructing that from job metadata every
-- time, which is why P-1 and P-2 had nothing to reference.
--
-- The archive's SHA-256 is the identity, not the filename or the URL. Cricsheet
-- reuses filenames across releases (all_json.zip is always all_json.zip), so a
-- filename-keyed registry would silently conflate every dataset ever fetched.

BEGIN;

CREATE TABLE IF NOT EXISTS public.datasets (
    id bigserial PRIMARY KEY,

    -- sha256 identifies the dataset. Unique so a re-fetch of unchanged bytes updates
    -- the existing row instead of adding a duplicate that means the same thing.
    sha256 character varying(64) NOT NULL UNIQUE,

    -- Where it came from. feed is null for an explicit URL, and both are null for an
    -- archive an operator placed in staging by hand -- a fact worth recording as
    -- null rather than papering over with a guess.
    feed character varying(64),
    source_url text,
    filename character varying(255) NOT NULL,
    bytes bigint NOT NULL DEFAULT 0,

    -- ETag and Last-Modified as the server reported them, so a later conditional
    -- fetch can be reasoned about after the fact.
    etag text,
    last_modified text,

    -- Fetch and extract are separate steps and either may be absent: an archive can
    -- be staged and never extracted, or extracted after being placed by hand.
    fetched_at timestamp with time zone,
    extracted_at timestamp with time zone,

    -- What the extract produced. entry_count is every file written; match_files is
    -- the subset the importer will read. They differ when an archive carries a
    -- README, and the difference is worth keeping.
    entry_count integer,
    match_files integer,
    extracted_bytes bigint,
    dest_dir text,

    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);

-- The registry is read newest-first by /ops/data/datasets.
CREATE INDEX IF NOT EXISTS datasets_created_at_idx ON public.datasets (created_at DESC);

-- Note: there is deliberately no is_live column. Which dataset is live is a property
-- of the filesystem, not of this table -- an operator who rsyncs files into the data
-- directory changes what is live without touching Postgres. It is derived by matching
-- the on-disk .dataset-manifest digest against sha256, so the answer cannot go stale.

COMMIT;
