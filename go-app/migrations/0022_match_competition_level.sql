-- 0022_match_competition_level.sql
--
-- Record the competition level of a match beside its format code (IMPORT-09 in
-- docs/AUDIT_FINDINGS.md).
--
-- The four format codes pool cricket that the archive tells apart. `match_format` has TEST,
-- ODI, T20 and T20I; Cricsheet's `match_type` has Test, ODI, T20, IT20, MDM and ODM, and
-- beside it `team_type` says whether the two sides are national teams ("international") or
-- not ("club"). The importer folds MDM into TEST and ODM into ODI, so on the archive as it
-- stands 2,207 of the 3,125 TEST rows (70.6 %) are Sheffield Shield, County Championship
-- and their kin, and 2,060 of the 5,242 ODI rows (39.3 %) are domestic one-day cricket.
-- `original_match_type` already recovers that split. What it could not recover was the
-- T20 / T20I one: Cricsheet writes every twenty-over match as "T20", so the importer
-- inferred T20I from a hand list of twelve team names, which took the 3,888 T20s between
-- other national sides -- World Cup qualifiers, and the 87 World Cup matches the twelve
-- played against them -- for club cricket.
--
-- Two columns, both nullable and both the archive's own words:
--
-- `competition_level` is `info.team_type` verbatim, one of 'international' or 'club'.
-- Every file in the archive carries one, and the importer refuses a file that does not.
-- The importer's T20I rule now reads it in place of the list; nothing else reads it yet.
-- It is what a decision on the pooling -- does TEST mean Test cricket or first-class
-- cricket? -- can be measured against and, once taken, applied with an UPDATE of
-- format_id rather than a re-import.
--
-- `match_type_number` is the ICC's running number for an official international of that
-- type. It is present on exactly the files that had official status: every Test and ODI,
-- and the 5,700 T20s the ICC counts as T20Is. It is absent from the 320 IT20 files, which
-- are internationals played before the sides had T20I status, and from every club match.
--
-- Nothing back-fills either: a re-import of the whole directory is what writes them, and
-- the same re-import is what moves the 3,888 matches the list misfiled onto T20I.

BEGIN;

ALTER TABLE public.match ADD COLUMN IF NOT EXISTS competition_level character varying(16)
    CONSTRAINT match_competition_level_check CHECK (competition_level IN ('international', 'club'));
ALTER TABLE public.match ADD COLUMN IF NOT EXISTS match_type_number integer;

COMMENT ON COLUMN public.match.competition_level IS
    'Cricsheet info.team_type verbatim (international | club): whether both sides are national teams. Null until the match is re-imported. Decides T20 vs T20I; MDM/ODM stay pooled with TEST/ODI (IMPORT-09)';
COMMENT ON COLUMN public.match.match_type_number IS
    'Cricsheet info.match_type_number: the ICC''s running number for an official Test, ODI or T20I; null for club matches, for internationals without official status (IT20) and until re-imported';

COMMIT;
