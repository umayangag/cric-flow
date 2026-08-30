/**
 * The model artifact kinds the ops console and health tab report on, in pipeline order.
 *
 * One list, imported by every reader. A kind the pipeline trains but a reader's private copy
 * of this list omits reads as "never trained" — which is how the innings model stayed
 * invisible in both `/health` and `/ops/status` after a completed training run.
 *
 * Mirrors `opsstatus.artifactKinds` (go-app) and `app.artifacts.ARTIFACT_KINDS` (ml-service).
 */
export const ARTIFACT_KINDS = [
  'batting',
  'bowling',
  'fielding',
  'extras',
  'win',
  'innings',
] as const;

export type ArtifactKind = (typeof ARTIFACT_KINDS)[number];

/** One cell of the artifacts matrix, as reported by go-app /ops/status. */
export type ArtifactUnit = {
  exists?: boolean;
  loaded?: boolean;
  /** The file on disk is newer than the object the ML service loaded from it. */
  stale?: boolean;
};
