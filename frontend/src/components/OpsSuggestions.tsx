import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import { Suggestion } from '../types';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';

const OpsSuggestions: React.FC = () => {
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.opsSuggestions();
      setSuggestions(data);
      setError(null);
    } catch (e) {
      if (e instanceof Error) {
        setError(e.message);
      } else {
        setError(String(e));
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    const interval = setInterval(load, 10000);
    return () => clearInterval(interval);
  }, [load]);

  const onCopy = async (idx: number, cmd: string) => {
    try {
      await navigator.clipboard.writeText(cmd);
      setCopiedIdx(idx);
      setTimeout(() => setCopiedIdx(null), 1200);
    } catch {
      setCopiedIdx(null);
      alert('Failed to copy to clipboard');
    }
  };

  if (loading && suggestions.length === 0) return <Typography variant="body2">Loading suggestions...</Typography>;
  if (error) return <Typography variant="body2" color="error">Error: {error}</Typography>;

  return (
    <Box sx={{ display: 'grid', gap: 2 }}>
      {suggestions.length === 0 ? (
        <Typography variant="body2" sx={{ opacity: 0.8 }}>No suggestions.</Typography>
      ) : (
        suggestions.map((s, i) => (
          <Box
            key={s.command || s.title}
            sx={{
              display: 'grid',
              gap: 1,
              pl: 1.5,
              borderLeft: '4px solid',
              borderColor: s.priority === 'HIGH' ? 'error.main' : 'primary.main',
              borderRadius: 1,
              py: 1.5,
              pr: 1.5,
              bgcolor: 'action.hover',
            }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1, flexWrap: 'wrap' }}>
              <Box>
                <Typography component="strong" variant="body2" fontWeight={600}>{s.title}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ fontSize: 13 }}>{s.description}</Typography>
              </Box>
              {s.command && (
                <Button
                  size="small"
                  variant="outlined"
                  onClick={() => onCopy(i, s.command)}
                  title="Copy command"
                  sx={{
                    textTransform: 'none',
                    borderColor: 'divider',
                    color: 'text.secondary',
                    '&:hover': { borderColor: 'primary.main', color: 'primary.main', bgcolor: 'action.hover' },
                  }}
                >
                  {copiedIdx === i ? 'Copied!' : 'Copy'}
                </Button>
              )}
            </Box>
            {s.command && (
              <Box
                component="pre"
                sx={{
                  m: 0,
                  p: 1.5,
                  bgcolor: 'grey.100',
                  color: 'text.primary',
                  borderRadius: 1,
                  fontFamily: 'ui-monospace, Menlo, monospace',
                  fontSize: 12,
                  overflowX: 'auto',
                  border: '1px solid',
                  borderColor: 'divider',
                  userSelect: 'all',
                  cursor: 'text',
                }}
              >
                {s.command}
              </Box>
            )}
          </Box>
        ))
      )}
    </Box>
  );
};

export default OpsSuggestions;
