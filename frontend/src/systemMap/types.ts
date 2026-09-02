/**
 * The shape of `contracts/system-map.json`.
 *
 * The contract is the single source of truth for the System map tab: the nodes, the
 * edges, the layout, the prose and the keys of every live value. Nothing about the
 * pipeline is described in this directory's components — they render what the contract
 * says, so a change to the map is a change to one reviewable file.
 *
 * `scripts/check-system-map.py` verifies the same file against the code in both
 * directions on every PR, which is what stops the map drifting from what it draws.
 */

/** How a live value is rendered. Mirrors VALUE_KINDS in scripts/check-system-map.py. */
export type ValueKind =
  | 'integer'
  | 'number1'
  | 'ratio3'
  | 'signed3'
  | 'percent'
  | 'bytes'
  | 'text'
  | 'sha'
  | 'date'
  | 'timestamp'
  | 'yes_no'
  | 'list';

/** Which endpoint a live value comes from. The tab already calls all three. */
export type SourceId = 'ops_status' | 'xi_status' | 'report';

export type SystemMapSource = {
  label: string;
  /** `"METHOD /path"`, asserted against the routes the services actually serve. */
  endpoint: string;
  note: string;
};

/**
 * One live value a node shows.
 *
 * No number is written into the contract — only the key and the path to read it at.
 * `per_format` paths carry `{format}` and are rendered once per format the report knows.
 */
export type SystemMapBinding = {
  key: string;
  label: string;
  source: SourceId;
  path: string;
  value_kind: ValueKind;
  per_format?: boolean;
  /**
   * The metric glossary key this value is explained by (L-1). When the report carries a
   * glossary, the value gets the explainer; the map holds no metric prose of its own.
   */
  glossary_key?: string;
};

/** Code the node points at. Every entry is checked to exist by the CI check. */
export type SystemMapAnchors = {
  /** Dotted ml-service module paths, e.g. `ml.xi.ratings`. */
  modules?: string[];
  /** Repo-relative Go package directories. */
  packages?: string[];
  /** Repo-relative files. */
  files?: string[];
  /** `"METHOD /path"` as the services serve it. */
  endpoints?: string[];
  /** Database tables a migration creates and none drops. */
  tables?: string[];
  /** Root Makefile targets. */
  make_targets?: string[];
  /** Files a run directory holds, named by the code that writes them. */
  artifacts?: string[];
  /** Ids from the ops pipeline step registry. */
  pipeline_steps?: string[];
  /** Ids from the H-23 gate registry. */
  gates?: string[];
  /** Column-family constants in `ml.xi.contract`. */
  feature_groups?: string[];
  /** Performance-model target names. */
  targets?: string[];
};

export type DocumentReference = { doc: string; section: string };

/** A node's inner structure, shown when the node is expanded. */
export type SystemMapChild = {
  id: string;
  label: string;
  summary: string;
  anchors?: SystemMapAnchors;
};

export type SystemMapNode = {
  id: string;
  label: string;
  lane: string;
  column: number;
  kind: 'data' | 'process' | 'model' | 'artifact' | 'service' | 'surface';
  summary: string;
  anchors: SystemMapAnchors;
  documented_in: DocumentReference[];
  bindings?: SystemMapBinding[];
  expandable?: boolean;
  children?: SystemMapChild[];
  /**
   * `"gates"` means the node's inner structure is the gate registry the report carries,
   * not a list written here: the map names the gate ids and the report supplies the
   * prose and the value, so the two cannot describe a gate differently.
   */
  children_from?: 'gates';
  children_note?: string;
};

export type SystemMapEdge = {
  from: string;
  to: string;
  label?: string;
  /** `control` is an orchestration arrow (who runs what), drawn dashed. */
  kind?: 'control';
};

export type SystemMapLane = { id: string; label: string; note: string };

export type SystemMapLayout = {
  node_width: number;
  node_height: number;
  column_gap: number;
  lane_gap: number;
  child_height: number;
  padding: number;
};

export type SystemMap = {
  version: number;
  layout: SystemMapLayout;
  sources: Record<SourceId, SystemMapSource>;
  lanes: SystemMapLane[];
  nodes: SystemMapNode[];
  edges: SystemMapEdge[];
};
