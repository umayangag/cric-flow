import React from 'react';
import { Box, Chip, Divider, Link, Paper, Stack, Typography } from '@mui/material';
import { MetricInfo } from './common/MetricInfo';
import { resolveBinding, resolveGates } from '../systemMap/bindings';
import type { BindingSources, ResolvedGate } from '../systemMap/bindings';
import { systemMap } from '../systemMap/contract';
import type { SystemMapAnchors, SystemMapNode } from '../systemMap/types';

/**
 * Everything the map knows about one step.
 *
 * Three kinds of thing, kept apart on purpose: the prose, which is curated and reviewed;
 * the anchors, which are the code this step *is* and which CI proves still exist; and the
 * live values, which are read from the endpoints on every load and are therefore the same
 * numbers the Ops and Evaluation tabs show, because they are literally those numbers.
 */

const ANCHOR_LABELS: Array<{ kind: keyof SystemMapAnchors; label: string }> = [
  { kind: 'modules', label: 'Python modules' },
  { kind: 'packages', label: 'Go packages' },
  { kind: 'files', label: 'Files' },
  { kind: 'endpoints', label: 'Endpoints' },
  { kind: 'tables', label: 'Tables' },
  { kind: 'make_targets', label: 'Make targets' },
  { kind: 'artifacts', label: 'Artifacts' },
  { kind: 'pipeline_steps', label: 'Pipeline steps' },
  { kind: 'gates', label: 'Gates' },
  { kind: 'feature_groups', label: 'Feature groups' },
  { kind: 'targets', label: 'Performance targets' },
];

const AnchorList: React.FC<{ anchors: SystemMapAnchors }> = ({ anchors }) => {
  const groups = ANCHOR_LABELS.filter(({ kind }) => (anchors[kind] ?? []).length > 0);
  if (groups.length === 0) return null;

  return (
    <Stack spacing={0.75}>
      {groups.map(({ kind, label }) => (
        <Box key={kind}>
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            {label}
          </Typography>
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mt: 0.25 }}>
            {(anchors[kind] ?? []).map((value) => (
              <Chip
                key={value}
                label={value}
                size="small"
                sx={{ fontFamily: 'monospace', fontSize: '0.7rem', bgcolor: 'action.hover' }}
              />
            ))}
          </Box>
        </Box>
      ))}
    </Stack>
  );
};

const GateList: React.FC<{ gates: ResolvedGate[]; note?: string }> = ({ gates, note }) => (
  <Stack spacing={1.25}>
    {note ? (
      <Typography variant="body2" sx={{ color: 'text.secondary' }}>
        {note}
      </Typography>
    ) : null}
    {gates.map((gate) => (
      <Paper key={gate.id} variant="outlined" sx={{ p: 1.25, boxShadow: 'none' }}>
        <Typography variant="subtitle2">
          {gate.id} — {gate.name}
        </Typography>
        <Typography variant="caption" component="div" sx={{ color: 'text.secondary' }}>
          <strong>Varies:</strong> {gate.varies}
        </Typography>
        <Typography variant="caption" component="div" sx={{ color: 'text.secondary' }}>
          <strong>Held fixed:</strong> {gate.fixed}
        </Typography>
        <Typography variant="caption" component="div" sx={{ color: 'text.secondary' }}>
          <strong>Decides:</strong> {gate.decides}
        </Typography>
        {gate.values.length > 0 ? (
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mt: 0.5 }}>
            {gate.values.map((value) => (
              <Chip
                key={`${gate.id}-${value.format}`}
                size="small"
                label={`${value.format}: ${value.text}`}
                sx={{ fontSize: '0.7rem' }}
              />
            ))}
          </Box>
        ) : null}
      </Paper>
    ))}
  </Stack>
);

type SystemMapNodeDetailProps = {
  node: SystemMapNode | null;
  sources: BindingSources;
};

const SystemMapNodeDetail: React.FC<SystemMapNodeDetailProps> = ({ node, sources }) => {
  if (!node) {
    return (
      <Paper variant="outlined" sx={{ p: 2, boxShadow: 'none' }}>
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          Pick a step on the map to read what it does, which code it is, and what it is currently
          reporting.
        </Typography>
      </Paper>
    );
  }

  const gateIds = node.children_from === 'gates' ? (node.anchors.gates ?? []) : [];
  const gates = gateIds.length > 0 ? resolveGates(gateIds, sources.report) : [];

  return (
    <Paper variant="outlined" sx={{ p: 2, boxShadow: 'none' }} data-testid="system-map-detail">
      <Stack spacing={1.5}>
        <Box>
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            {systemMap.lanes.find((lane) => lane.id === node.lane)?.label}
          </Typography>
          <Typography variant="h6">{node.label}</Typography>
        </Box>

        <Typography variant="body2">{node.summary}</Typography>

        {(node.bindings ?? []).length > 0 ? (
          <>
            <Divider />
            <Box>
              <Typography variant="subtitle2" gutterBottom>
                Right now
              </Typography>
              <Stack spacing={0.5}>
                {(node.bindings ?? []).flatMap((binding) =>
                  resolveBinding(binding, sources).map((value) => (
                    <Stack
                      key={value.key}
                      direction="row"
                      spacing={1}
                      alignItems="baseline"
                      justifyContent="space-between"
                    >
                      <Stack direction="row" spacing={0.5} alignItems="center" sx={{ minWidth: 0 }}>
                        <Typography variant="caption" sx={{ color: 'text.secondary' }} noWrap>
                          {value.format ? `${value.label} · ${value.format}` : value.label}
                        </Typography>
                        {value.glossaryKey ? (
                          <MetricInfo
                            metricKey={value.glossaryKey}
                            label={value.label}
                            value={value.text}
                          />
                        ) : null}
                      </Stack>
                      <Typography
                        variant="body2"
                        sx={{
                          fontFamily: 'monospace',
                          fontWeight: 600,
                          color: value.present ? 'text.primary' : 'text.disabled',
                          textAlign: 'right',
                        }}
                      >
                        {value.text}
                      </Typography>
                    </Stack>
                  )),
                )}
              </Stack>
            </Box>
          </>
        ) : null}

        {(node.children ?? []).length > 0 ? (
          <>
            <Divider />
            <Box>
              <Typography variant="subtitle2" gutterBottom>
                Inside this step
              </Typography>
              <Stack spacing={1.25}>
                {(node.children ?? []).map((child) => (
                  <Paper key={child.id} variant="outlined" sx={{ p: 1.25, boxShadow: 'none' }}>
                    <Typography variant="subtitle2">{child.label}</Typography>
                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                      {child.summary}
                    </Typography>
                    {child.anchors ? (
                      <Box sx={{ mt: 0.75 }}>
                        <AnchorList anchors={child.anchors} />
                      </Box>
                    ) : null}
                  </Paper>
                ))}
              </Stack>
            </Box>
          </>
        ) : null}

        {gates.length > 0 ? (
          <>
            <Divider />
            <Box>
              <Typography variant="subtitle2" gutterBottom>
                The gates it checks
              </Typography>
              <GateList gates={gates} note={node.children_note} />
            </Box>
          </>
        ) : null}

        <Divider />
        <Box>
          <Typography variant="subtitle2" gutterBottom>
            The code this is
          </Typography>
          <AnchorList anchors={node.anchors} />
        </Box>

        <Divider />
        <Box>
          <Typography variant="subtitle2" gutterBottom>
            Documented in
          </Typography>
          <Stack spacing={0.25}>
            {node.documented_in.map((reference) => (
              <Link
                key={`${reference.doc}${reference.section}`}
                href={`https://github.com/umayangag/cric-flow/blob/main/${reference.doc}`}
                target="_blank"
                rel="noreferrer"
                variant="body2"
              >
                {reference.doc} — {reference.section.replace(/^#+\s*/, '')}
              </Link>
            ))}
          </Stack>
        </Box>
      </Stack>
    </Paper>
  );
};

export default SystemMapNodeDetail;
