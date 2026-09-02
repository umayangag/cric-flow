import { describe, it, expect } from 'vitest';
import { systemMap } from './contract';
import { layout } from './layout';
import type { ValueKind } from './types';
import { formatValue, MISSING } from './bindings';

/**
 * The frontend half of the system-map contract.
 *
 * `scripts/check-system-map.py` checks the map against the code — that every module,
 * endpoint, table, gate and make target it names exists, and that nothing the code can
 * enumerate is missing from it. That check knows nothing about this app. These are the
 * invariants the renderer relies on: that the graph is well formed, that the layout puts
 * no two boxes in one slot, and that every way of rendering a value the contract asks for
 * is a way this code actually has.
 */

const VALUE_KINDS: ValueKind[] = [
  'integer',
  'number1',
  'ratio3',
  'signed3',
  'percent',
  'bytes',
  'text',
  'sha',
  'date',
  'timestamp',
  'yes_no',
  'list',
];

describe('the system map contract', () => {
  it('gives every node a lane that exists', () => {
    const lanes = new Set(systemMap.lanes.map((lane) => lane.id));
    const orphans = systemMap.nodes.filter((node) => !lanes.has(node.lane)).map((node) => node.id);
    expect(orphans).toEqual([]);
  });

  it('never puts two nodes in the same lane and column', () => {
    const slots = systemMap.nodes.map((node) => `${node.lane}:${node.column}`);
    expect(new Set(slots).size).toBe(slots.length);
  });

  it('draws every edge between nodes that exist', () => {
    const ids = new Set(systemMap.nodes.map((node) => node.id));
    const dangling = systemMap.edges
      .filter((edge) => !ids.has(edge.from) || !ids.has(edge.to))
      .map((edge) => `${edge.from} -> ${edge.to}`);
    expect(dangling).toEqual([]);
  });

  it('gives every node prose a non-expert can read and a document to go on with', () => {
    for (const node of systemMap.nodes) {
      expect(node.summary.length, `${node.id} summary`).toBeGreaterThan(120);
      expect(node.documented_in.length, `${node.id} documented_in`).toBeGreaterThan(0);
    }
  });

  it('reads every live value from a source the tab actually calls', () => {
    const sources = new Set(Object.keys(systemMap.sources));
    for (const node of systemMap.nodes) {
      for (const binding of node.bindings ?? []) {
        expect(sources.has(binding.source), `${node.id}.${binding.key}`).toBe(true);
      }
    }
  });

  /**
   * The one that would otherwise fail silently: a value_kind the contract asks for and
   * the formatter has no case for would render as `String(value)` and look almost right.
   */
  it('asks only for ways of rendering a value that this code has', () => {
    const kinds = new Set<string>();
    for (const node of systemMap.nodes) {
      for (const binding of node.bindings ?? []) kinds.add(binding.value_kind);
    }
    for (const kind of kinds) {
      expect(VALUE_KINDS, `value_kind ${kind}`).toContain(kind);
    }
  });

  it('renders nothing as a dash for every kind of value', () => {
    for (const kind of VALUE_KINDS) {
      expect(formatValue(undefined, kind), kind).toBe(MISSING);
    }
  });

  it('marks a node expandable exactly when it has inner structure', () => {
    for (const node of systemMap.nodes) {
      const hasChildren = (node.children ?? []).length > 0 || node.children_from === 'gates';
      expect(Boolean(node.expandable), `${node.id} expandable`).toBe(hasChildren);
    }
  });
});

describe('the system map layout', () => {
  it('places every node inside the canvas it reports', () => {
    const placement = layout(new Set(), {});
    expect(placement.nodes).toHaveLength(systemMap.nodes.length);
    for (const placed of placement.nodes) {
      expect(placed.x + placed.width, placed.node.id).toBeLessThanOrEqual(placement.width);
      expect(placed.y + placed.height, placed.node.id).toBeLessThanOrEqual(placement.height);
    }
  });

  /**
   * The layout risk that a screenshot would catch and a unit test usually would not:
   * two boxes on top of each other. Checked with everything expanded, which is the
   * tallest the graph ever gets.
   */
  it('never overlaps two boxes, collapsed or fully expanded', () => {
    const expandable = systemMap.nodes.filter((node) => node.expandable).map((node) => node.id);
    const childCounts = Object.fromEntries(
      systemMap.nodes.map((node) => [node.id, Math.max(node.children?.length ?? 0, 10)]),
    );

    for (const expanded of [new Set<string>(), new Set(expandable)]) {
      const placed = layout(expanded, childCounts).nodes;
      for (let i = 0; i < placed.length; i++) {
        for (let j = i + 1; j < placed.length; j++) {
          const a = placed[i];
          const b = placed[j];
          const overlaps =
            a.x < b.x + b.width &&
            b.x < a.x + a.width &&
            a.y < b.y + b.height &&
            b.y < a.y + a.height;
          expect(overlaps, `${a.node.id} overlaps ${b.node.id}`).toBe(false);
        }
      }
    }
  });

  it('grows the lane so an expanded node cannot cover the lane below it', () => {
    const collapsed = layout(new Set(), { 'rating-pass': 3 });
    const expanded = layout(new Set(['rating-pass']), { 'rating-pass': 3 });
    expect(expanded.height).toBeGreaterThan(collapsed.height);

    const laneIndex = systemMap.lanes.findIndex((lane) => lane.id === 'l1');
    const above = expanded.lanes[laneIndex];
    const below = expanded.lanes[laneIndex + 1];
    expect(above.y + above.height).toBeLessThan(below.y);
  });
});
