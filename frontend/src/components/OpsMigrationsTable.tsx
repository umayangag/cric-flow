import React, { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../api';
import type { AutoTuneRunDetailsEntry } from '../types';
import { Migration } from '../types';
import AutoTuneRunCard from './AutoTuneRunCard';
import StatusPill from './common/StatusPill';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import { Box } from '@mui/material';
import Chip from '@mui/material/Chip';
import Typography from '@mui/material/Typography';
import JsonCollapse from './common/JsonCollapse';
import RunSummaryPanel from './RunSummaryPanel';
import { asRunMetadata, findPreviousRun, flattenMetrics, parseFailure } from '../utils/runMetadata';
import {
  TableContainer,
  Table,
  TableHead,
  TableHeaderCell,
  TableRow,
  TableCell,
  CodeCell,
  DetailsCell,
  ErrorText,
  EmptyStateCell,
  PaginationContainer,
  PaginationButton,
  ViewDetailsButton,
} from './OpsMigrationsTable.styles';

// Helper to format duration or time ago
function formatTimeAgo(dateStr: string) {
  const date = new Date(dateStr);
  const now = new Date();
  const diff = (now.getTime() - date.getTime()) / 1000;

  if (diff < 60) return `${Math.floor(diff)}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

function formatDuration(start: string, end?: string) {
  if (!end) return '-';
  const s = new Date(start).getTime();
  const e = new Date(end).getTime();
  const ms = e - s;
  if (ms < 1000) return `${ms}ms`;
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  return `${min}m ${sec % 60}s`;
}

const getPillState = (status: Migration['status']): 'ok' | 'pending' | 'error' => {
  switch (status) {
    case 'COMPLETED':
      return 'ok';
    case 'IN_PROGRESS':
      return 'pending';
    default:
      return 'error';
  }
};

/** Format cutoff (ISO string or Unix timestamp) for display. */
function formatCutoff(v: unknown): string {
  if (v == null) return '';
  const d = typeof v === 'number' ? new Date(v >= 1e10 ? v : v * 1000) : new Date(String(v));
  return isNaN(d.getTime()) ? String(v).slice(0, 10) : d.toISOString().slice(0, 10);
}

type ArgFormatter = (args: Record<string, unknown>) => string[];

const CMD_FORMATTERS: Record<string, ArgFormatter> = {
  'ml-auto-tune': (args) => {
    const parts: string[] = [];
    if (args.model) parts.push(`model=${args.model}`);
    if (args.format) parts.push(`format=${args.format}`);
    if (args.all_formats && String(args.all_formats) !== '0') parts.push('all_formats');
    if (args.algorithms) parts.push(`algorithms=${args.algorithms}`);
    if (args.cutoff) parts.push(`cutoff=${formatCutoff(args.cutoff)}`);
    if (args.rescreen && String(args.rescreen) !== '0') parts.push('rescreen');
    return parts;
  },
  'export-dataset': (args) => (args.out_dir ? [`out=${String(args.out_dir)}`] : []),
  'precompute-features': (args) => (args.season ? [`season=${args.season}`] : []),
  'cricsheet-import': (args) => (args.dir ? [`dir=${String(args.dir)}`] : []),
};

/** Format command + params for display in the Command column (model, format, algorithms, cutoff, etc.) */
function formatCommandWithParams(m: Migration): string {
  const cmd = m.command || '';
  const args = (m.args as Record<string, unknown>) || {};

  let formatter = CMD_FORMATTERS[cmd];
  if (!formatter && cmd.startsWith('train-')) {
    formatter = (a) => (a.cutoff ? [`cutoff=${formatCutoff(a.cutoff)}`] : []);
  }
  const parts = formatter ? formatter(args) : [];

  if (parts.length === 0) return cmd;
  return `${cmd} ${parts.join(' ')}`;
}

/**
 * A failed run's message, with the code and the next action pulled out.
 *
 * go-app already formats an ml-service precondition as `CODE: message — hint` so the
 * operator is told what to do next (C5-2's CONTRIBUTIONS_CSV_MISSING is the model).
 * This is presentation only: it stops the hint being buried mid-way through a red
 * monospace block, and falls back to the raw string when there is no structure.
 */
const FailureDetails: React.FC<{ errorMessage: string }> = ({ errorMessage }) => {
  const failure = parseFailure(errorMessage);
  if (!failure) return null;

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
      {failure.code && <Chip size="small" color="error" label={failure.code} />}
      <Typography
        variant="body2"
        color="error"
        component="div"
        sx={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}
      >
        {failure.message}
      </Typography>
      {failure.hint && (
        <Box
          sx={{
            p: 1.5,
            borderRadius: 1,
            bgcolor: 'warning.light',
            color: 'warning.contrastText',
          }}
        >
          <Typography variant="body2">
            <strong>Next:</strong> {failure.hint}
          </Typography>
        </Box>
      )}
    </Box>
  );
};

/** How many recent runs to search when looking for the previous run of a step. */
const COMPARISON_WINDOW = 100;

const OpsMigrationsTable: React.FC = () => {
  const [migrations, setMigrations] = useState<Migration[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [detailsMigration, setDetailsMigration] = useState<Migration | null>(null);
  const [autoTuneRuns, setAutoTuneRuns] = useState<AutoTuneRunDetailsEntry[] | null>(null);
  const [detailsLoading, setDetailsLoading] = useState(false);
  const [detailsError, setDetailsError] = useState<string | null>(null);
  const detailsAbortRef = useRef<AbortController | null>(null);
  const [previousRun, setPreviousRun] = useState<Migration | undefined>();
  const [comparisonReady, setComparisonReady] = useState(false);
  const limit = 10;

  const load = useCallback(
    async (isPolling = false) => {
      if (!isPolling) setLoading(true);
      try {
        const data = await api.opsMigrations(page, limit);
        setMigrations(data.items || []);
        setTotal(data.total);
        setError(null);
      } catch (e) {
        if (e instanceof Error) {
          setError(e.message);
        } else {
          setError(String(e));
        }
      } finally {
        if (!isPolling) setLoading(false);
      }
    },
    [page],
  );

  useEffect(() => {
    load(false);
    const interval = setInterval(() => load(true), 5000); // Poll every 5s
    return () => clearInterval(interval);
  }, [load]);

  const totalPages = Math.max(1, Math.ceil((total ?? 0) / limit));

  const handleViewDetails = async (m: Migration) => {
    detailsAbortRef.current?.abort();
    const ac = new AbortController();
    detailsAbortRef.current = ac;

    setDetailsMigration(m);
    setDetailsError(null);
    setAutoTuneRuns(null);
    setPreviousRun(undefined);
    setComparisonReady(false);

    // A metric only means something next to the last one, and the previous run of
    // this step is usually not on the page being viewed. One bounded request when
    // the dialog opens beats loading a hundred rows the table never shows.
    if (m.status === 'COMPLETED') {
      void api
        .opsMigrations(1, COMPARISON_WINDOW)
        .then((window) => {
          if (ac.signal.aborted) return;
          setPreviousRun(findPreviousRun(m, window.items ?? []));
        })
        .catch(() => {
          // A failed comparison lookup is not a failed drill-down: the run's own
          // detail is already here, and "no comparison available" is the honest
          // rendering of not knowing.
        })
        .finally(() => {
          if (!ac.signal.aborted) setComparisonReady(true);
        });
    } else {
      setComparisonReady(true);
    }

    if (m.command === 'ml-auto-tune' && m.status === 'COMPLETED') {
      setDetailsLoading(true);
      try {
        const res = await api.autoTuneDetailsForMigration(m.id, { signal: ac.signal });
        setAutoTuneRuns(res.runs ?? []);
      } catch (e) {
        if (ac.signal.aborted) return;
        const msg = e instanceof Error ? e.message : String(e);
        setDetailsError(msg);
      } finally {
        if (detailsAbortRef.current === ac) {
          setDetailsLoading(false);
        }
      }
    } else {
      setDetailsLoading(false);
    }
  };

  const closeDetails = () => {
    detailsAbortRef.current?.abort();
    detailsAbortRef.current = null;
    setDetailsMigration(null);
    setAutoTuneRuns(null);
    setDetailsError(null);
    setPreviousRun(undefined);
    setComparisonReady(false);
    setDetailsLoading(false);
  };

  const runMetadata = asRunMetadata(detailsMigration?.metadata);
  const previousMetrics = previousRun
    ? flattenMetrics(asRunMetadata(previousRun.metadata))
    : undefined;

  if (loading && migrations.length === 0) return <div>Loading migrations...</div>;
  if (error) return <ErrorText>Error: {error}</ErrorText>;

  return (
    <TableContainer>
      <Table>
        <TableHead>
          <tr>
            <TableHeaderCell>ID</TableHeaderCell>
            <TableHeaderCell>Command</TableHeaderCell>
            <TableHeaderCell>Status</TableHeaderCell>
            <TableHeaderCell>Started</TableHeaderCell>
            <TableHeaderCell>Duration</TableHeaderCell>
            <TableHeaderCell>Details</TableHeaderCell>
          </tr>
        </TableHead>
        <tbody>
          {migrations.map((m) => (
            <TableRow key={m.id}>
              <TableCell>{m.id}</TableCell>
              <CodeCell title={typeof m.args === 'object' ? JSON.stringify(m.args) : undefined}>
                {formatCommandWithParams(m)}
              </CodeCell>
              <TableCell>
                <StatusPill state={getPillState(m.status)} label={m.status} />
              </TableCell>
              <TableCell>{formatTimeAgo(m.started_at)}</TableCell>
              <TableCell>{formatDuration(m.started_at, m.completed_at)}</TableCell>
              <DetailsCell>
                <ViewDetailsButton
                  type="button"
                  onClick={() => void handleViewDetails(m)}
                  aria-label={`View details for migration ${m.id}`}
                >
                  View details
                </ViewDetailsButton>
              </DetailsCell>
            </TableRow>
          ))}
          {migrations.length === 0 && (
            <tr>
              <EmptyStateCell colSpan={6}>No commands executed yet.</EmptyStateCell>
            </tr>
          )}
        </tbody>
      </Table>
      <PaginationContainer>
        <PaginationButton disabled={page === 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
          Newer
        </PaginationButton>
        <span>
          Page {page} of {totalPages}
        </span>
        <PaginationButton disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
          Older
        </PaginationButton>
      </PaginationContainer>

      <Dialog
        open={detailsMigration !== null}
        onClose={closeDetails}
        maxWidth="sm"
        fullWidth
        PaperProps={{ sx: { maxHeight: '80vh' } }}
      >
        <DialogTitle>
          Details{' '}
          {detailsMigration != null
            ? `— ${detailsMigration.command} (ID ${detailsMigration.id})`
            : ''}
        </DialogTitle>
        <DialogContent dividers>
          {detailsMigration != null && detailsMigration.error_message && (
            <FailureDetails errorMessage={detailsMigration.error_message} />
          )}

          {detailsMigration != null &&
            !detailsMigration.error_message &&
            detailsMigration.command === 'ml-auto-tune' &&
            detailsMigration.status === 'COMPLETED' && (
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                {detailsLoading && <div>Loading auto-tune results…</div>}
                {detailsError && (
                  <ErrorText>Error loading auto-tune details: {detailsError}</ErrorText>
                )}
                {!detailsLoading &&
                  !detailsError &&
                  (!autoTuneRuns || autoTuneRuns.length === 0) && (
                    <div>No auto-tune results were recorded for this migration.</div>
                  )}
                {!detailsLoading &&
                  !detailsError &&
                  autoTuneRuns &&
                  autoTuneRuns.length > 0 &&
                  autoTuneRuns.map((run) => <AutoTuneRunCard key={run.id} run={run} />)}
              </Box>
            )}

          {detailsMigration != null &&
            !detailsMigration.error_message &&
            (detailsMigration.command !== 'ml-auto-tune' ||
              detailsMigration.status !== 'COMPLETED') && (
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                {runMetadata ? (
                  <RunSummaryPanel
                    metadata={runMetadata}
                    previousMetrics={previousMetrics}
                    previousRun={previousRun}
                    comparisonReady={comparisonReady}
                  />
                ) : (
                  // Steps go-app runs itself record their own shapes, and rows written
                  // before O-4 record almost nothing. Raw JSON is the honest rendering
                  // for both: there is no structure here to present.
                  <Typography variant="body2" color="text.secondary">
                    This run recorded no structured summary.
                  </Typography>
                )}
                <JsonCollapse
                  data={{ args: detailsMigration.args, metadata: detailsMigration.metadata }}
                  summary="Show raw JSON"
                />
              </Box>
            )}
        </DialogContent>
        <DialogActions>
          <Button onClick={closeDetails}>Close</Button>
        </DialogActions>
      </Dialog>
    </TableContainer>
  );
};

export default OpsMigrationsTable;
