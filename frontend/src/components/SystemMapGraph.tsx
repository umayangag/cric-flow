import React, { useCallback, useMemo, useRef, useState } from 'react';
import { Box, IconButton, Stack, Tooltip, Typography, useTheme } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import RemoveIcon from '@mui/icons-material/Remove';
import CenterFocusStrongIcon from '@mui/icons-material/CenterFocusStrong';
import { systemMap } from '../systemMap/contract';
import { edgePath, layout } from '../systemMap/layout';
import type { PlacedNode } from '../systemMap/layout';
import type { SystemMapNode } from '../systemMap/types';

/**
 * The map itself: hand-laid boxes and curves, no graph library.
 *
 * `reactflow` is already in the app (the ops format hierarchy uses it) so this is not a
 * bundle argument. It is a control argument. The layout has to be deterministic from the
 * contract — a moved node must be a reviewable diff, not the output of a force pass —
 * every node has to be a real focusable button so the whole map is reachable by keyboard,
 * and the boxes have to grow when a node is expanded. All three are easier to do than to
 * argue a library out of. Zoom is a CSS transform and panning is the container's own
 * scroll, so a focused node scrolls into view without any code of ours.
 */

const MIN_ZOOM = 0.5;
const MAX_ZOOM = 1.6;
const ZOOM_STEP = 0.15;

type SystemMapGraphProps = {
  selectedId: string | null;
  onSelect: (id: string) => void;
  expandedIds: ReadonlySet<string>;
  onToggleExpand: (id: string) => void;
  /** Inner-structure labels per node id: the contract's children, or the report's gates. */
  childLabels: Record<string, string[]>;
};

/** Node colour by what kind of thing it is, so the eye can group lanes at a glance. */
function kindColor(kind: SystemMapNode['kind']): string {
  switch (kind) {
    case 'data':
      return 'info.main';
    case 'model':
      return 'secondary.main';
    case 'artifact':
      return 'warning.main';
    case 'service':
      return 'success.main';
    case 'surface':
      return 'primary.dark';
    case 'process':
    default:
      return 'primary.main';
  }
}

const SystemMapGraph: React.FC<SystemMapGraphProps> = ({
  selectedId,
  onSelect,
  expandedIds,
  onToggleExpand,
  childLabels,
}) => {
  const theme = useTheme();
  const [zoom, setZoom] = useState(1);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const drag = useRef<{ x: number; y: number; left: number; top: number } | null>(null);

  const childCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const [id, labels] of Object.entries(childLabels)) counts[id] = labels.length;
    return counts;
  }, [childLabels]);

  const placement = useMemo(() => layout(expandedIds, childCounts), [expandedIds, childCounts]);
  const placedById = useMemo(
    () => new Map<string, PlacedNode>(placement.nodes.map((placed) => [placed.node.id, placed])),
    [placement],
  );

  const onPointerDown = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    const container = scrollRef.current;
    if (!container || (event.target as HTMLElement).closest('button')) return;
    drag.current = {
      x: event.clientX,
      y: event.clientY,
      left: container.scrollLeft,
      top: container.scrollTop,
    };
  }, []);

  const onPointerMove = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    const container = scrollRef.current;
    if (!container || !drag.current) return;
    container.scrollLeft = drag.current.left - (event.clientX - drag.current.x);
    container.scrollTop = drag.current.top - (event.clientY - drag.current.y);
  }, []);

  const endDrag = useCallback(() => {
    drag.current = null;
  }, []);

  const changeZoom = (delta: number) =>
    setZoom((current) =>
      Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Number((current + delta).toFixed(2)))),
    );

  const edgeColor = theme.palette.divider;
  const edgeLabelColor = theme.palette.text.secondary;
  const paper = theme.palette.background.paper;

  return (
    <Box>
      <Stack direction="row" spacing={0.5} alignItems="center" sx={{ mb: 1 }}>
        <Tooltip title="Zoom out">
          <span>
            <IconButton
              size="small"
              onClick={() => changeZoom(-ZOOM_STEP)}
              disabled={zoom <= MIN_ZOOM}
            >
              <RemoveIcon fontSize="small" />
            </IconButton>
          </span>
        </Tooltip>
        <Typography
          variant="caption"
          sx={{ minWidth: 44, textAlign: 'center', color: 'text.secondary' }}
        >
          {Math.round(zoom * 100)} %
        </Typography>
        <Tooltip title="Zoom in">
          <span>
            <IconButton
              size="small"
              onClick={() => changeZoom(ZOOM_STEP)}
              disabled={zoom >= MAX_ZOOM}
            >
              <AddIcon fontSize="small" />
            </IconButton>
          </span>
        </Tooltip>
        <Tooltip title="Reset the view">
          <IconButton
            size="small"
            aria-label="Reset the view"
            onClick={() => {
              setZoom(1);
              if (scrollRef.current) scrollRef.current.scrollTo({ left: 0, top: 0 });
            }}
          >
            <CenterFocusStrongIcon fontSize="small" />
          </IconButton>
        </Tooltip>
        <Typography variant="caption" sx={{ color: 'text.secondary', pl: 1 }}>
          Drag to pan · click a step for its details · Tab moves through every step
        </Typography>
      </Stack>

      <Box
        ref={scrollRef}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endDrag}
        onPointerLeave={endDrag}
        sx={{
          position: 'relative',
          overflow: 'auto',
          height: { xs: 420, md: 620 },
          border: '1px solid',
          borderColor: 'divider',
          borderRadius: 2,
          bgcolor: 'background.default',
          touchAction: 'pan-x pan-y',
        }}
      >
        <Box
          sx={{
            width: placement.width * zoom,
            height: placement.height * zoom,
            position: 'relative',
          }}
        >
          <Box
            sx={{
              position: 'absolute',
              top: 0,
              left: 0,
              width: placement.width,
              height: placement.height,
              transform: `scale(${zoom})`,
              transformOrigin: '0 0',
            }}
          >
            <svg
              width={placement.width}
              height={placement.height}
              style={{ position: 'absolute', inset: 0 }}
              aria-hidden
            >
              <defs>
                <marker
                  id="system-map-arrow"
                  markerWidth="9"
                  markerHeight="9"
                  refX="7"
                  refY="3"
                  orient="auto"
                >
                  <path d="M0,0 L0,6 L7,3 z" fill={edgeColor} />
                </marker>
              </defs>
              {placement.lanes.map((lane) => (
                <rect
                  key={lane.id}
                  x={systemMap.layout.padding / 2}
                  y={lane.y - 6}
                  width={placement.width - systemMap.layout.padding}
                  height={lane.height + 12}
                  rx={10}
                  fill={theme.palette.action.hover}
                />
              ))}
              {systemMap.edges.map((edge) => {
                const path = edgePath(edge, placedById);
                if (!path) return null;
                return (
                  <g key={`${edge.from}->${edge.to}`}>
                    <path
                      d={path.d}
                      fill="none"
                      stroke={edgeColor}
                      strokeWidth={1.5}
                      strokeDasharray={edge.kind === 'control' ? '5 4' : undefined}
                      markerEnd="url(#system-map-arrow)"
                    />
                    {edge.label ? (
                      <text
                        x={path.mid[0]}
                        y={path.mid[1]}
                        fontSize={10}
                        textAnchor="middle"
                        fill={edgeLabelColor}
                        stroke={paper}
                        strokeWidth={4}
                        paintOrder="stroke"
                      >
                        {edge.label}
                      </text>
                    ) : null}
                  </g>
                );
              })}
            </svg>

            {placement.lanes.map((lane) => (
              <Typography
                key={lane.id}
                variant="caption"
                sx={{
                  position: 'absolute',
                  left: systemMap.layout.padding,
                  top: lane.y - 22,
                  color: 'text.secondary',
                  fontWeight: 600,
                  letterSpacing: '0.04em',
                  textTransform: 'uppercase',
                  fontSize: 10,
                }}
              >
                {lane.label}
              </Typography>
            ))}

            {placement.nodes.map(({ node, x, y, width, height }) => {
              const expanded = expandedIds.has(node.id);
              const labels = childLabels[node.id] ?? [];
              const selected = selectedId === node.id;
              return (
                <Box
                  key={node.id}
                  sx={{ position: 'absolute', left: x, top: y, width, height }}
                  data-testid={`system-map-node-${node.id}`}
                >
                  <Box
                    component="button"
                    type="button"
                    onClick={() => onSelect(node.id)}
                    aria-pressed={selected}
                    sx={{
                      width: '100%',
                      height: '100%',
                      textAlign: 'left',
                      display: 'block',
                      p: 1,
                      cursor: 'pointer',
                      font: 'inherit',
                      borderRadius: 2,
                      border: '1px solid',
                      borderColor: selected ? 'primary.main' : 'divider',
                      borderLeft: '4px solid',
                      borderLeftColor: kindColor(node.kind),
                      bgcolor: 'background.paper',
                      boxShadow: selected ? 3 : 1,
                      transition: 'box-shadow .15s, border-color .15s',
                      '&:hover': { borderColor: 'primary.main', boxShadow: 2 },
                      '&:focus-visible': {
                        outline: '2px solid',
                        outlineColor: 'primary.main',
                        outlineOffset: 2,
                      },
                    }}
                  >
                    <Typography variant="body2" fontWeight={600} sx={{ lineHeight: 1.2 }}>
                      {node.label}
                    </Typography>
                    <Typography
                      variant="caption"
                      sx={{
                        color: 'text.secondary',
                        display: '-webkit-box',
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: 'vertical',
                        overflow: 'hidden',
                        mt: 0.25,
                      }}
                    >
                      {node.summary}
                    </Typography>
                    {expanded ? (
                      <Box component="ul" sx={{ m: 0, mt: 0.5, pl: 2 }}>
                        {labels.map((label) => (
                          <Typography
                            key={label}
                            component="li"
                            variant="caption"
                            sx={{
                              color: 'text.secondary',
                              height: systemMap.layout.child_height,
                              lineHeight: `${systemMap.layout.child_height}px`,
                              whiteSpace: 'nowrap',
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                            }}
                          >
                            {label}
                          </Typography>
                        ))}
                      </Box>
                    ) : null}
                  </Box>

                  {node.expandable ? (
                    <IconButton
                      size="small"
                      aria-label={`${expanded ? 'Collapse' : 'Expand'} ${node.label}`}
                      aria-expanded={expanded}
                      onClick={() => onToggleExpand(node.id)}
                      sx={{ position: 'absolute', top: 2, right: 2, p: 0.25 }}
                    >
                      {expanded ? (
                        <RemoveIcon sx={{ fontSize: 14 }} />
                      ) : (
                        <AddIcon sx={{ fontSize: 14 }} />
                      )}
                    </IconButton>
                  ) : null}
                </Box>
              );
            })}
          </Box>
        </Box>
      </Box>
    </Box>
  );
};

export default SystemMapGraph;
