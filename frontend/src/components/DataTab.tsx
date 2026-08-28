import React from 'react';
import { Box } from '@mui/material';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import DataFetchSection from './DataFetchSection';
import DataExtractSection from './DataExtractSection';
import DatasetRegistryTable from './DatasetRegistryTable';
import ErrorNotice from './common/ErrorNotice';
import PipelineProgressPanel from './PipelineProgressPanel';
import { useDataTab } from '../hooks/useDataTab';

/**
 * Acquiring a dataset without a terminal: fetch, extract, and what has been acquired.
 *
 * This is its own tab rather than a section of Ops Status because acquisition is what
 * you do *before* the pipeline, not a stage of it — the same distinction the backend
 * encodes as Step.Surface, so the pipeline graph does not show fetch and extract.
 *
 * Live progress reuses PipelineProgressPanel and the one SSE stream everything else
 * uses. A second progress mechanism for these two steps would be a second thing to
 * connect, reconnect and keep in sync for no gain.
 */
const DataTab: React.FC = () => {
  const { feeds, staged, registry, busy, error, loading, refresh, markStarted } = useDataTab();

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
        <Typography variant="h5" sx={{ flexGrow: 1 }}>
          Data
        </Typography>
        {loading && <CircularProgress size={18} />}
        <Button size="small" variant="outlined" onClick={refresh} disabled={loading}>
          Refresh
        </Button>
      </Box>

      <ErrorNotice error={error} title="Could not load dataset state" />

      <PipelineProgressPanel pipelineRunning={busy} onRefresh={refresh} />

      <Box
        sx={{
          display: 'grid',
          gap: 2,
          gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' },
          alignItems: 'start',
        }}
      >
        <DataFetchSection feeds={feeds} busy={busy} onStarted={markStarted} />
        <DataExtractSection staged={staged} busy={busy} onStarted={markStarted} />
      </Box>

      <DatasetRegistryTable registry={registry} />

      <Typography variant="caption" color="text.secondary">
        Once extracted, run <strong>Import</strong> from Ops Status to load the match files into the
        database. Chaining fetch → extract → import into one action is a later step in the plan
        (R-1).
      </Typography>
    </Box>
  );
};

export default DataTab;
