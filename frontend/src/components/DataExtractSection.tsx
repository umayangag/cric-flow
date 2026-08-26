import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Box } from '@mui/material';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import MenuItem from '@mui/material/MenuItem';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import SectionCard from './common/SectionCard';
import { api } from '../api';
import { formatBytes } from './OpsDatasetSection';
import { formatWhen, shortDigest } from '../utils/datasetFormat';
import type { StagedResponse } from '../types';

type Props = {
  staged?: StagedResponse;
  /** True while any data-lane step is running; extracting is refused with 409 then. */
  busy: boolean;
  onStarted: () => void;
};

/**
 * Extract a staged archive into the dataset directory.
 *
 * The warning is not decoration: extraction *replaces* the dataset directory rather
 * than merging into it, so a stale match file from the previous dataset cannot
 * survive. That is the right behaviour and it is also destructive, which makes it
 * something to say before the click rather than explain afterwards.
 */
const DataExtractSection: React.FC<Props> = ({ staged, busy, onStarted }) => {
  // Memoised so the default-selection effect below does not re-run on every render:
  // `?? []` builds a fresh array each time, which is a new dependency each time.
  const archives = useMemo(() => staged?.archives ?? [], [staged]);
  const [archive, setArchive] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  // Default to the newest archive, which is what the backend picks for an empty
  // request. Showing the same choice it would make beats an empty box that means
  // something the operator cannot see.
  useEffect(() => {
    if (archive === '' && archives.length > 0) setArchive(archives[0].filename);
  }, [archives, archive]);

  const handleSubmit = useCallback(async () => {
    setError(null);
    setNotice(null);
    setSubmitting(true);
    try {
      const { status, data } = await api.opsDataExtract(archive ? { archive } : {});
      if (status === 202) {
        setNotice(`Extracting ${data.archive ?? archive}…`);
        onStarted();
        return;
      }
      setError(data.error || `Failed (${status})`);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Request failed');
    } finally {
      setSubmitting(false);
    }
  }, [archive, onStarted]);

  const selected = archives.find((a) => a.filename === archive);

  return (
    <SectionCard
      title="Extract a dataset"
      subtitle="Unpacks a staged archive into the dataset directory Import reads from."
    >
      {archives.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          Nothing staged. Fetch an archive first, or place a <code>.zip</code> in{' '}
          <code>{staged?.staging_dir || 'the staging directory'}</code>.
        </Typography>
      ) : (
        <>
          <TextField
            select
            size="small"
            label="Archive"
            value={archive}
            onChange={(e) => setArchive(e.target.value)}
            SelectProps={{ MenuProps: { disableScrollLock: true } }}
          >
            {archives.map((a) => (
              <MenuItem key={a.filename} value={a.filename}>
                {a.filename} · {formatBytes(a.bytes)} · {formatWhen(a.modified)}
              </MenuItem>
            ))}
          </TextField>

          {selected && (
            <Typography variant="caption" color="text.secondary" component="div">
              {selected.source_url ? (
                <>
                  From <strong>{selected.feed_id || 'URL'}</strong> · {selected.source_url}
                </>
              ) : (
                <>Placed here by hand — no recorded source.</>
              )}
              {selected.sha256 && <> · sha256 {shortDigest(selected.sha256)}</>}
            </Typography>
          )}

          <Alert severity="warning">
            Extracting <strong>replaces</strong> the dataset directory — match files not in this
            archive will be removed. The previous contents are kept under the staging directory, so
            this is recoverable.
          </Alert>

          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
            <Button
              variant="contained"
              size="small"
              color="warning"
              disabled={busy || submitting || archive === ''}
              onClick={handleSubmit}
            >
              {submitting ? 'Starting…' : 'Extract'}
            </Button>
            {busy && (
              <Typography variant="caption" color="text.secondary">
                A dataset step is already running.
              </Typography>
            )}
          </Box>
        </>
      )}

      {notice && <Alert severity="success">{notice}</Alert>}
      {error && <Alert severity="error">{error}</Alert>}
    </SectionCard>
  );
};

export default DataExtractSection;
