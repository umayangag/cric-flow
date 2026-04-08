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
    if (args.unified && String(args.unified) !== '0') parts.push('unified');
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
    setDetailsLoading(false);
  };

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
            <Box
              component="pre"
              sx={{
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
                maxHeight: '60vh',
                overflow: 'auto',
                margin: 0,
                fontSize: 12,
                color: '#dc2626',
                fontFamily: 'ui-monospace, Menlo, monospace',
              }}
            >
              {detailsMigration.error_message}
            </Box>
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
              <Box
                component="pre"
                sx={{
                  p: 1.5,
                  bgcolor: 'grey.100',
                  color: 'text.primary',
                  borderRadius: 1,
                  overflow: 'auto',
                  maxHeight: '60vh',
                  border: '1px solid',
                  borderColor: 'divider',
                  fontSize: 12,
                  fontFamily: 'ui-monospace, Menlo, monospace',
                  margin: 0,
                }}
              >
                {JSON.stringify(
                  {
                    args: detailsMigration.args,
                    meta: detailsMigration.metadata,
                  },
                  null,
                  2,
                )}
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
