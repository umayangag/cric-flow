import rawContract from '../../../contracts/system-map.json';
import type { SystemMap, SystemMapNode } from './types';

/**
 * The system map, loaded from the contract itself rather than copied into the frontend.
 *
 * `contracts/ops-console.contract.json` is mirrored in TypeScript because it is a short
 * list of ids and a test can assert the two agree line by line. This one is a graph with
 * paragraphs of prose on every node; a mirrored copy would be the drift it exists to
 * prevent, so the JSON is imported directly and bundled. Vite's `server.fs.allow` is
 * widened to the repo root for the dev server for the same reason.
 */
export const systemMap = rawContract as unknown as SystemMap;

export const systemMapNodes: SystemMapNode[] = systemMap.nodes;

export function nodeById(id: string): SystemMapNode | undefined {
  return systemMap.nodes.find((node) => node.id === id);
}

/** Lane ids in drawing order, so a lane's index is its row. */
export const laneOrder: string[] = systemMap.lanes.map((lane) => lane.id);
