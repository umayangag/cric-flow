import React, { useState } from 'react';
import type { MLModelStat } from '../types';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import Chip from '@mui/material/Chip';
import Collapse from '@mui/material/Collapse';
import IconButton from '@mui/material/IconButton';
import KeyboardArrowDown from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUp from '@mui/icons-material/KeyboardArrowUp';
import MLModelRowDetails from './MLModelRowDetails';

function formatBytes(n: number | undefined): string {
  if (n == null || !isFinite(n) || n <= 0) return '—';
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}

function formatModified(iso: string | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  const pad = (num: number) => num.toString().padStart(2, '0');
  const date = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  return `${date} ${time}`;
}

function formatDuration(seconds: number | undefined): string {
  if (seconds == null || !isFinite(seconds) || seconds < 0) return '—';
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const m = Math.floor(seconds / 60);
  const s = Math.round(seconds % 60);
  return s > 0 ? `${m}m ${s}s` : `${m}m`;
}

function getAccuracyDisplay(model: MLModelStat): string {
  if (model.accuracy_display) return model.accuracy_display;
  if (model.best_cv_score != null && isFinite(model.best_cv_score)) {
    const scoring = (model.scoring || '').toLowerCase();
    if (scoring.includes('neg_mean_absolute_error') || scoring.includes('neg_mae')) {
      const mae = Math.abs(model.best_cv_score).toFixed(2);
      return model.scoring ? `MAE=${mae} (${model.scoring})` : `MAE=${mae}`;
    }
    const pct = (model.best_cv_score * 100).toFixed(1);
    return model.scoring ? `${pct}% (${model.scoring})` : `${pct}%`;
  }
  return '—';
}

function statusColor(status: 'PASS' | 'FAIL' | 'WARNING'): 'success' | 'error' | 'warning' {
  if (status === 'PASS') return 'success';
  if (status === 'FAIL') return 'error';
  return 'warning';
}

export function MLModelRow({ model }: { model: MLModelStat }) {
  const [open, setOpen] = useState(false);
  const hasDetails =
    (model.tuned_parameters && Object.keys(model.tuned_parameters).length > 0) ||
    (model.metrics && Object.keys(model.metrics).length > 0) ||
    (model.feature_importance && Object.keys(model.feature_importance).length > 0) ||
    (model.mlqa_audit && (model.mlqa_audit.key_findings?.length > 0 || model.mlqa_audit.final_verdict));

  return (
    <>
      <TableRow sx={{ '& > *': { borderBottom: 'unset' } }}>
        <TableCell>
          {hasDetails && (
            <IconButton
              aria-label={open ? 'hide details' : 'show details'}
              size="small"
              onClick={() => setOpen(!open)}
            >
              {open ? <KeyboardArrowUp /> : <KeyboardArrowDown />}
            </IconButton>
          )}
        </TableCell>
        <TableCell component="th" scope="row">{model.model_name}</TableCell>
        <TableCell><Chip label={model.match_format} size="small" variant="outlined" /></TableCell>
        <TableCell>{model.algorithm ?? '—'}</TableCell>
        <TableCell>{getAccuracyDisplay(model)}</TableCell>
        <TableCell>
          {model.mlqa_audit ? (
            <Chip label={model.mlqa_audit.audit_status} size="small" color={statusColor(model.mlqa_audit.audit_status)} />
          ) : '—'}
        </TableCell>
        <TableCell>{formatBytes(model.size_bytes)}</TableCell>
        <TableCell sx={{ fontSize: '0.85rem' }}>{formatModified(model.modified)}</TableCell>
        <TableCell>{model.tuned ? 'Yes' : 'No'}</TableCell>
        <TableCell sx={{ fontSize: '0.85rem' }}>{formatModified(model.trained_at)}</TableCell>
        <TableCell>{formatDuration(model.duration_seconds)}</TableCell>
      </TableRow>
      {hasDetails && (
        <TableRow>
          <TableCell sx={{ py: 0 }} colSpan={11}>
            <Collapse in={open} timeout="auto" unmountOnExit>
              <MLModelRowDetails model={model} />
            </Collapse>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

export default MLModelRow;
