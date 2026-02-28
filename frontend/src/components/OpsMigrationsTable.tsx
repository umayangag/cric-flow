import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import { Migration } from '../types';
import StatusPill from './common/StatusPill';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
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

/** Format command + params for display in the Command column (model, format, algorithms, cutoff, etc.) */
function formatCommandWithParams(m: Migration): string {
  const cmd = m.command || '';
  const args = (m.args as Record<string, unknown>) || {};
  const parts: string[] = [];

  if (cmd === 'ml-auto-tune') {
    if (args.model) parts.push(`model=${args.model}`);
    if (args.format) parts.push(`format=${args.format}`);
    if (args.all_formats && String(args.all_formats) !== '0') parts.push('all_formats');
    if (args.unified && String(args.unified) !== '0') parts.push('unified');
    if (args.algorithms) parts.push(`algorithms=${args.algorithms}`);
    if (args.cutoff) parts.push(`cutoff=${String(args.cutoff).slice(0, 10)}`);
    if (args.rescreen && String(args.rescreen) !== '0') parts.push('rescreen');
  } else if (cmd.startsWith('train-')) {
    if (args.cutoff) parts.push(`cutoff=${String(args.cutoff).slice(0, 10)}`);
  } else if (cmd === 'export-dataset') {
    if (args.out_dir) parts.push(`out=${String(args.out_dir)}`);
  } else if (cmd === 'precompute-features') {
    if (args.season) parts.push(`season=${args.season}`);
  } else if (cmd === 'cricsheet-import') {
    if (args.dir) parts.push(`dir=${String(args.dir)}`);
  }

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
                  onClick={() => setDetailsMigration(m)}
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
        onClose={() => setDetailsMigration(null)}
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
          {detailsMigration != null &&
            (detailsMigration.error_message ? (
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
            ) : (
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
            ))}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDetailsMigration(null)}>Close</Button>
        </DialogActions>
      </Dialog>
    </TableContainer>
  );
};

export default OpsMigrationsTable;
