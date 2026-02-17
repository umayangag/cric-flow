import React from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

type MatrixType = 'precompute' | 'exports' | 'artifacts';

type PrecomputeData = {
  formats?: Record<string, { status?: string } | undefined>;
};
type ExportFile = { name?: string; exists?: boolean };
type ExportsData = {
  formats?: Record<string, { files?: ExportFile[] } | undefined>;
};
type ArtifactUnit = { exists?: boolean; loaded?: boolean };
type ArtifactsData = {
  formats?: Record<string, { batting?: ArtifactUnit; bowling?: ArtifactUnit } | undefined>;
};

type Props = {
  type: MatrixType;
  title: string;
  data: PrecomputeData | ExportsData | ArtifactsData;
};

const FORMATS = ['TEST', 'ODI', 'T20I', 'T20'] as const;

const cellSx = (state: 'ok' | 'stale' | 'error' | 'neutral') => ({
  borderRadius: 2,
  px: 1.25,
  py: 1,
  minWidth: 90,
  textAlign: 'center' as const,
  border: '1px solid',
  fontSize: 12,
  ...(state === 'ok' && {
    bgcolor: 'rgba(34, 197, 94, 0.1)',
    borderColor: 'rgba(34, 197, 94, 0.25)',
    color: 'success.dark',
  }),
  ...(state === 'stale' && {
    bgcolor: 'rgba(245, 158, 11, 0.1)',
    borderColor: 'rgba(245, 158, 11, 0.3)',
    color: 'warning.dark',
  }),
  ...(state === 'error' && {
    bgcolor: 'rgba(239, 68, 68, 0.08)',
    borderColor: 'rgba(239, 68, 68, 0.25)',
    color: 'error.dark',
  }),
  ...(state === 'neutral' && {
    bgcolor: 'grey.100',
    borderColor: 'divider',
    color: 'text.secondary',
  }),
});

export const OpsMatrix: React.FC<Props> = ({ type, title, data }) => {
  const renderPrecompute = () => (
    <Box sx={{ display: 'grid', gap: 1.5 }}>
      <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'wrap' }}>
        {FORMATS.map((f) => {
          const st = (data as PrecomputeData)?.formats?.[f]?.status as string | undefined;
          const state =
            st === 'ok' ? 'ok' : st === 'stale' ? 'stale' : st === 'missing' ? 'error' : 'neutral';
          const txt = st ?? 'unknown';
          return (
            <Box
              key={f}
              sx={cellSx(state)}
              title={`status: ${txt}`}
              data-testid={`precompute-${f}`}
            >
              <Typography component="strong" variant="body2" fontWeight={600}>
                {f}
              </Typography>
              <Typography variant="caption" display="block" sx={{ fontSize: 11, opacity: 0.95 }}>
                {txt}
              </Typography>
            </Box>
          );
        })}
      </Box>
      <Box
        sx={{
          display: 'flex',
          gap: 1.5,
          alignItems: 'center',
          fontSize: 11,
          color: 'text.secondary',
        }}
      >
        <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
          <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: 'success.main' }} />
          ok
        </Box>
        <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
          <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: 'warning.main' }} />
          stale
        </Box>
        <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
          <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: 'error.main' }} />
          missing
        </Box>
      </Box>
    </Box>
  );

  const hasAnyExists = (files: unknown): boolean => {
    if (!Array.isArray(files)) return false;
    return files.some((e) => !!(e && typeof e === 'object' && (e as ExportFile).exists === true));
  };

  const renderExports = () => (
    <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'wrap' }}>
      {FORMATS.map((f) => {
        const files = (data as ExportsData)?.formats?.[f]?.files ?? [];
        const ok = hasAnyExists(files);
        const state = ok ? 'ok' : 'error';
        const titleStr = Array.isArray(files)
          ? files
              .map((x) => (x && typeof x === 'object' ? (x as ExportFile).name : undefined))
              .filter(Boolean)
              .join(', ')
          : '';
        return (
          <Box key={f} sx={cellSx(state)} title={titleStr} data-testid={`exports-${f}`}>
            <Typography component="strong" variant="body2" fontWeight={600}>
              {f}
            </Typography>
            <Typography variant="caption" display="block" sx={{ fontSize: 11, opacity: 0.95 }}>
              {ok ? 'present' : 'missing'}
            </Typography>
          </Box>
        );
      })}
    </Box>
  );

  const subCell = (ok: boolean, loaded?: boolean) => (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
      <Box
        sx={{
          width: 10,
          height: 10,
          borderRadius: 1,
          bgcolor: ok ? 'success.main' : 'error.main',
          opacity: ok ? 1 : 0.85,
        }}
      />
      <Typography variant="caption" sx={{ fontSize: 11, color: 'text.secondary' }}>
        {ok ? 'exists' : 'missing'}
      </Typography>
      {loaded && (
        <Box
          title="loaded"
          sx={{ width: 6, height: 6, bgcolor: 'success.light', borderRadius: '50%' }}
        />
      )}
    </Box>
  );

  const renderArtifacts = () => (
    <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'wrap' }}>
      {FORMATS.map((f) => {
        const bat = (data as ArtifactsData)?.formats?.[f]?.batting ?? {};
        const bowl = (data as ArtifactsData)?.formats?.[f]?.bowling ?? {};
        const ok = bat?.exists === true && bowl?.exists === true;
        const state = ok ? 'ok' : 'error';
        return (
          <Box key={f} sx={cellSx(state)} data-testid={`artifacts-${f}`}>
            <Typography component="strong" variant="body2" fontWeight={600}>
              {f}
            </Typography>
            <Box sx={{ display: 'grid', gap: 0.5, mt: 0.75 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                {subCell(bat?.exists === true, bat?.loaded === true)}
                <Typography
                  component="small"
                  variant="caption"
                  sx={{ fontSize: 10, color: 'text.secondary' }}
                >
                  batting
                </Typography>
              </Box>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                {subCell(bowl?.exists === true, bowl?.loaded === true)}
                <Typography
                  component="small"
                  variant="caption"
                  sx={{ fontSize: 10, color: 'text.secondary' }}
                >
                  bowling
                </Typography>
              </Box>
            </Box>
          </Box>
        );
      })}
    </Box>
  );

  return (
    <Box component="section" sx={{ mt: 0.5 }}>
      <Typography variant="subtitle2" fontWeight={600} sx={{ mb: 1 }}>
        {title}
      </Typography>
      {type === 'precompute' && renderPrecompute()}
      {type === 'exports' && renderExports()}
      {type === 'artifacts' && renderArtifacts()}
    </Box>
  );
};

export default OpsMatrix;
