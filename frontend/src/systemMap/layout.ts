import { systemMap } from './contract';
import type { SystemMapEdge, SystemMapNode } from './types';

/**
 * Where every box goes.
 *
 * Positions are derived from the contract's lane and column, never stored as pixels and
 * never computed by a force-directed pass: the same contract always draws the same
 * picture, and moving a node is a one-line diff someone can review. A lane grows taller
 * when a node in it is expanded, so opening one node never puts it on top of another.
 */

export type PlacedNode = {
  node: SystemMapNode;
  x: number;
  y: number;
  width: number;
  height: number;
};

export type PlacedLane = { id: string; label: string; note: string; y: number; height: number };

export type Placement = {
  nodes: PlacedNode[];
  lanes: PlacedLane[];
  width: number;
  height: number;
};

/** How much taller an expanded node is than a collapsed one. */
export function expandedExtra(node: SystemMapNode, childCount: number): number {
  if (childCount === 0) return 0;
  return childCount * systemMap.layout.child_height + 16;
}

/**
 * Lay the graph out.
 *
 * `childCounts` is passed in rather than read from the contract because the harness's
 * children are the gates the report carries, which are not known until it loads.
 */
export function layout(
  expandedIds: ReadonlySet<string>,
  childCounts: Record<string, number>,
): Placement {
  const { node_width, node_height, column_gap, lane_gap, padding } = systemMap.layout;

  const laneHeights = systemMap.lanes.map((lane) => {
    const extras = systemMap.nodes
      .filter((node) => node.lane === lane.id && expandedIds.has(node.id))
      .map((node) => expandedExtra(node, childCounts[node.id] ?? 0));
    return node_height + Math.max(0, ...extras);
  });

  const lanes: PlacedLane[] = [];
  let cursor = padding;
  systemMap.lanes.forEach((lane, index) => {
    lanes.push({
      id: lane.id,
      label: lane.label,
      note: lane.note,
      y: cursor,
      height: laneHeights[index],
    });
    cursor += laneHeights[index] + lane_gap;
  });

  const laneY = new Map(lanes.map((lane) => [lane.id, lane.y]));
  const nodes: PlacedNode[] = systemMap.nodes.map((node) => ({
    node,
    x: padding + node.column * (node_width + column_gap),
    y: laneY.get(node.lane) ?? padding,
    width: node_width,
    height:
      node_height + (expandedIds.has(node.id) ? expandedExtra(node, childCounts[node.id] ?? 0) : 0),
  }));

  const maxColumn = Math.max(...systemMap.nodes.map((node) => node.column));
  return {
    nodes,
    lanes,
    width: padding * 2 + (maxColumn + 1) * node_width + maxColumn * column_gap,
    height: cursor - lane_gap + padding,
  };
}

/**
 * The path for one arrow.
 *
 * Down a lane the arrow leaves the bottom and enters the top; along a lane it leaves the
 * right and enters the left; an orchestration arrow that points back up a lane leaves the
 * top. Curves rather than elbows, because an elbow router that has to avoid boxes is a
 * dependency, and this graph is small enough that it does not need one.
 */
export function edgePath(
  edge: SystemMapEdge,
  placed: Map<string, PlacedNode>,
): { d: string; mid: [number, number] } | null {
  const from = placed.get(edge.from);
  const to = placed.get(edge.to);
  if (!from || !to) return null;

  const fromLane = systemMap.lanes.findIndex((lane) => lane.id === from.node.lane);
  const toLane = systemMap.lanes.findIndex((lane) => lane.id === to.node.lane);

  let start: [number, number];
  let end: [number, number];
  let control: [number, number];

  if (toLane > fromLane) {
    start = [from.x + from.width / 2, from.y + from.height];
    end = [to.x + to.width / 2, to.y];
    control = [start[0], (start[1] + end[1]) / 2];
  } else if (toLane < fromLane) {
    start = [from.x + from.width / 2, from.y];
    end = [to.x + to.width / 2, to.y + to.height];
    control = [end[0], (start[1] + end[1]) / 2];
  } else if (to.x >= from.x) {
    start = [from.x + from.width, from.y + from.height / 2];
    end = [to.x, to.y + to.height / 2];
    control = [(start[0] + end[0]) / 2, start[1]];
  } else {
    start = [from.x, from.y + from.height / 2];
    end = [to.x + to.width, to.y + to.height / 2];
    control = [(start[0] + end[0]) / 2, start[1]];
  }

  const mid: [number, number] = [
    (start[0] + 2 * control[0] + end[0]) / 4,
    (start[1] + 2 * control[1] + end[1]) / 4,
  ];
  return { d: `M ${start[0]} ${start[1]} Q ${control[0]} ${control[1]} ${end[0]} ${end[1]}`, mid };
}
