import React, { useState } from 'react';

type Suggestion = {
  reason?: string;
  commands?: string[];
};

type Props = { suggestions: Suggestion[] | undefined };

const codeStyle: React.CSSProperties = {
  background: '#111',
  color: '#ddd',
  padding: 12,
  borderRadius: 6,
  fontFamily: 'ui-monospace, Menlo, monospace',
  fontSize: 12,
  overflowX: 'auto',
  border: '1px solid rgba(255,255,255,0.06)',
};

export const OpsSuggestions: React.FC<Props> = ({ suggestions }) => {
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null);

  const onCopy = async (idx: number, cmds: string[]) => {
    try {
      await navigator.clipboard.writeText(cmds.join(' && '));
      setCopiedIdx(idx);
      setTimeout(() => setCopiedIdx(null), 1200);
    } catch {
      setCopiedIdx(null);
      alert('Failed to copy to clipboard');
    }
  };

  const list = Array.isArray(suggestions) ? suggestions : [];
  return (
    <section>
      <h3 style={{ margin: '8px 0' }}>Suggestions</h3>
      {list.length === 0 ? (
        <div style={{ opacity: 0.9 }}>No suggestions. All systems look ready.</div>
      ) : (
        <div style={{ display: 'grid', gap: 12 }}>
          {list.map((s, i) => {
            const cmds = Array.isArray(s.commands) ? s.commands : [];
            return (
              <div key={i} style={{ display: 'grid', gap: 6 }}>
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: 8,
                  }}
                >
                  <strong>{s.reason || 'Suggestion'}</strong>
                  {cmds.length > 0 && (
                    <button
                      type="button"
                      onClick={() => onCopy(i, cmds)}
                      title="Copy commands"
                      aria-label={`Copy commands: ${cmds.join(' && ')}`}
                    >
                      {copiedIdx === i ? 'Copied!' : 'Copy'}
                    </button>
                  )}
                </div>
                {cmds.length > 0 && <pre style={codeStyle}>{cmds.join('\n')}</pre>}
              </div>
            );
          })}
        </div>
      )}
      <div style={{ marginTop: 8 }}>
        <a href="/docs/ops-status.md" target="_blank" rel="noreferrer" style={{ fontSize: 12 }}>
          Read the full Ops Status docs
        </a>
      </div>
    </section>
  );
};

export default OpsSuggestions;
