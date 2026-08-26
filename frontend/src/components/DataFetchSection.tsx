import React, { useCallback, useMemo, useState } from 'react';
import { Box } from '@mui/material';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import MenuItem from '@mui/material/MenuItem';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import SectionCard from './common/SectionCard';
import { api } from '../api';
import type { DataFeedsResponse } from '../types';

/** Which source the operator is naming. The backend refuses both at once. */
type SourceMode = 'feed' | 'url';

type Props = {
  feeds?: DataFeedsResponse;
  /** True while any data-lane step is running; fetching is refused with 409 then. */
  busy: boolean;
  onStarted: () => void;
};

/**
 * Fetch a Cricsheet archive into staging.
 *
 * The allowlist is stated up front rather than left to be discovered by a rejection:
 * an operator who pastes a URL should be able to see beforehand whether it can work.
 * Feed and URL are mutually exclusive because the backend treats supplying both as an
 * ambiguity to refuse, not a default to resolve — so the UI offers a choice, not two
 * boxes that can silently disagree.
 */
const DataFetchSection: React.FC<Props> = ({ feeds, busy, onStarted }) => {
  const [mode, setMode] = useState<SourceMode>('feed');
  const [feedId, setFeedId] = useState('');
  const [url, setUrl] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [allowedHosts, setAllowedHosts] = useState<string[] | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  // Memoised for the same reason as the selected-feed lookup below: a fresh `?? []`
  // on every render would defeat the memo it feeds.
  const feedList = useMemo(() => feeds?.feeds ?? [], [feeds]);
  const hosts = allowedHosts ?? feeds?.allowed_hosts ?? [];
  const selectedFeed = useMemo(() => feedList.find((f) => f.id === feedId), [feedList, feedId]);

  const canSubmit = !busy && !submitting && (mode === 'feed' ? feedId !== '' : url.trim() !== '');

  const handleSubmit = useCallback(async () => {
    setError(null);
    setNotice(null);
    setAllowedHosts(null);
    setSubmitting(true);
    try {
      const body = mode === 'feed' ? { feed: feedId } : { url: url.trim() };
      const { status, data } = await api.opsDataFetch(body);
      if (status === 202) {
        setNotice(`Downloading ${data.filename ?? data.feed ?? 'archive'}…`);
        onStarted();
        return;
      }
      setError(data.error || `Failed (${status})`);
      // A refusal that names what *would* be accepted beats one that only says no.
      if (data.allowed_hosts) setAllowedHosts(data.allowed_hosts);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Request failed');
    } finally {
      setSubmitting(false);
    }
  }, [mode, feedId, url, onStarted]);

  return (
    <SectionCard
      title="Fetch a dataset"
      subtitle="Downloads a Cricsheet archive into the staging directory. Nothing is written to the live dataset directory until you extract it."
    >
      <ToggleButtonGroup
        size="small"
        exclusive
        value={mode}
        onChange={(_, next: SourceMode | null) => next && setMode(next)}
        aria-label="Archive source"
      >
        <ToggleButton value="feed">Named feed</ToggleButton>
        <ToggleButton value="url">Explicit URL</ToggleButton>
      </ToggleButtonGroup>

      {mode === 'feed' ? (
        <TextField
          select
          size="small"
          label="Feed"
          value={feedId}
          onChange={(e) => setFeedId(e.target.value)}
          disabled={feedList.length === 0}
          helperText={
            selectedFeed?.description ??
            (feedList.length === 0 ? 'Feeds unavailable' : 'Which archive to download')
          }
          SelectProps={{ MenuProps: { disableScrollLock: true } }}
        >
          {feedList.map((feed) => (
            <MenuItem key={feed.id} value={feed.id}>
              {feed.label}
            </MenuItem>
          ))}
        </TextField>
      ) : (
        <TextField
          size="small"
          label="Archive URL"
          placeholder="https://cricsheet.org/downloads/all_json.zip"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          helperText="Must be https, must end in .zip, and must be on the allowlist below."
        />
      )}

      <Typography variant="caption" color="text.secondary" component="div">
        The server will only download from:{' '}
        <strong>{hosts.length > 0 ? hosts.join(', ') : '—'}</strong>. Redirects that leave the
        allowlist are refused too.
      </Typography>

      {feeds?.staging_dir && (
        <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-all' }}>
          Staging directory: <code>{feeds.staging_dir}</code>
        </Typography>
      )}

      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
        <Button variant="contained" size="small" disabled={!canSubmit} onClick={handleSubmit}>
          {submitting ? 'Starting…' : 'Fetch'}
        </Button>
        {busy && (
          <Typography variant="caption" color="text.secondary">
            A dataset step is already running.
          </Typography>
        )}
      </Box>

      {notice && <Alert severity="success">{notice}</Alert>}
      {error && (
        <Alert severity="error">
          {error}
          {allowedHosts && allowedHosts.length > 0 && (
            <Box component="div" sx={{ mt: 0.5 }}>
              Allowed hosts: {allowedHosts.join(', ')}
            </Box>
          )}
        </Alert>
      )}
    </SectionCard>
  );
};

export default DataFetchSection;
