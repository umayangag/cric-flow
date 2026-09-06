import React, { useCallback, useMemo, useState } from 'react';
import { Box, Button, Chip, Grid, Stack, Typography } from '@mui/material';
import { Refresh as RefreshIcon } from '@mui/icons-material';
import { api } from '../api';
import { useAsync } from '../hooks/useAsync';
import ErrorNotice from './common/ErrorNotice';
import SystemMapGraph from './SystemMapGraph';
import SystemMapNodeDetail from './SystemMapNodeDetail';
import { systemMap, nodeById } from '../systemMap/contract';
import { resolveGates } from '../systemMap/bindings';
import type { BindingSources } from '../systemMap/bindings';

/**
 * The System map tab: the whole pipeline, from the Cricsheet zip to the prediction.
 *
 * Two rules hold it together and they are worth stating where someone will read them.
 *
 * **Structure cannot drift.** Every box, arrow, lane and code anchor comes from
 * `contracts/system-map.json`, and `make check-system-map` proves on every PR that
 * everything the map names still exists and that everything the code can enumerate is on
 * the map. Delete an endpoint and CI fails; it does not quietly fade from the diagram.
 *
 * **Numbers are not written down.** Nodes carry keys, not figures. They are resolved here
 * from /ops/status, /api/ml/xi-status and the evaluation report — the same three calls the
 * Ops, Workbench and Evaluation tabs make — so a number on this map is that endpoint's
 * number. A key the endpoints do not carry reads as a dash.
 *
 * It is deliberately read-only. The Ops Status tab's step graph is the control surface
 * that starts and stops runs; this one describes the system rather than driving it.
 */
const SystemMapTab: React.FC = () => {
  const opsStatus = useAsync(api.opsStatus, {
    runOnMount: [],
    errorMessage: 'Could not load ops status',
  });
  const xiStatus = useAsync(api.xiStatus, {
    runOnMount: [],
    errorMessage: 'Could not load the XI status',
  });
  const report = useAsync(api.evaluationReport, {
    runOnMount: [],
    errorMessage: 'Could not load the evaluation report',
  });

  const [selectedId, setSelectedId] = useState<string | null>(systemMap.nodes[0]?.id ?? null);
  const [expandedIds, setExpandedIds] = useState<ReadonlySet<string>>(() => new Set<string>());

  const sources: BindingSources = useMemo(
    () => ({ ops_status: opsStatus.data, xi_status: xiStatus.data, report: report.data }),
    [opsStatus.data, xiStatus.data, report.data],
  );

  /**
   * What each expandable node lists inside itself. The contract supplies its own children;
   * the harness's are the gate registry the report carries, so the map names the ids and
   * the report names the gates.
   */
  const childLabels = useMemo(() => {
    const labels: Record<string, string[]> = {};
    for (const node of systemMap.nodes) {
      if (node.children_from === 'gates') {
        labels[node.id] = resolveGates(node.anchors.gates ?? [], sources.report).map(
          (gate) => `${gate.id} — ${gate.name}`,
        );
      } else if (node.children?.length) {
        labels[node.id] = node.children.map((child) => child.label);
      }
    }
    return labels;
  }, [sources.report]);

  const toggleExpand = useCallback((id: string) => {
    setExpandedIds((current) => {
      const next = new Set(current);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  }, []);

  const refresh = useCallback(() => {
    void opsStatus.run();
    void xiStatus.run();
    void report.run();
  }, [opsStatus, xiStatus, report]);

  const selected = selectedId ? (nodeById(selectedId) ?? null) : null;
  const loading = opsStatus.loading || xiStatus.loading || report.loading;

  return (
    <Box>
      <Stack
        direction={{ xs: 'column', sm: 'row' }}
        spacing={1}
        alignItems={{ sm: 'center' }}
        justifyContent="space-between"
        sx={{ mb: 1.5 }}
      >
        <Box>
          <Typography variant="h6">System map</Typography>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            Every step from the Cricsheet archive to the prediction on screen. The shape of this map
            is checked against the code on every change; the numbers on it are read live from the
            same endpoints the other tabs use, so nothing here is written down twice.
          </Typography>
        </Box>
        <Button size="small" startIcon={<RefreshIcon />} onClick={refresh} disabled={loading}>
          Refresh
        </Button>
      </Stack>

      <Stack direction="row" spacing={1} sx={{ mb: 1.5, flexWrap: 'wrap', gap: 0.5 }}>
        {Object.entries(systemMap.sources).map(([id, source]) => (
          <Chip
            key={id}
            size="small"
            label={`${source.label}: ${source.endpoint}`}
            variant="outlined"
            sx={{ fontFamily: 'monospace', fontSize: '0.7rem' }}
          />
        ))}
      </Stack>

      {/*
        A failed call is reported and the map still draws: the structure is in the bundle,
        so a service that is down costs the reader the numbers, not the diagram.
      */}
      <ErrorNotice error={opsStatus.error} context="Ops status" />
      <ErrorNotice error={xiStatus.error} context="XI status" />
      <ErrorNotice error={report.error} context="Evaluation report" />

      <Grid container spacing={2}>
        <Grid item xs={12} md={8}>
          <SystemMapGraph
            selectedId={selectedId}
            onSelect={setSelectedId}
            expandedIds={expandedIds}
            onToggleExpand={toggleExpand}
            childLabels={childLabels}
          />
        </Grid>
        <Grid item xs={12} md={4}>
          <SystemMapNodeDetail node={selected} sources={sources} />
        </Grid>
      </Grid>
    </Box>
  );
};

export default SystemMapTab;
