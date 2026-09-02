import { describe, it, expect, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MLModelStatsSection } from './MLModelStatsSection';
import type { ModelStatsResponse } from '../types';

const stats: ModelStatsResponse = {
  models_dir: '/output/ml-service',
  models: [
    {
      model_kind: 'win',
      model_name: 'Win',
      match_format: 'T20',
      size_bytes: 2048,
      modified: '2026-08-30T10:00:00Z',
      tuned: true,
      algorithm: 'gb',
      accuracy_display: '74.7%',
    },
    {
      model_kind: 'win',
      model_name: 'Win',
      match_format: 'ODI',
      size_bytes: 1024,
      modified: '2026-08-29T10:00:00Z',
    },
  ],
};

describe('MLModelStatsSection', () => {
  it('lists every loaded model with the directory it came from', () => {
    render(<MLModelStatsSection data={stats} error={null} loading={false} onRefresh={vi.fn()} />);

    expect(screen.getByText('/output/ml-service')).toBeInTheDocument();
    expect(screen.getAllByText('Win').length).toBeGreaterThan(0);
    expect(screen.getByText('T20')).toBeInTheDocument();
    expect(screen.getByText('ODI')).toBeInTheDocument();
  });

  it('refreshes on request', () => {
    const onRefresh = vi.fn();
    render(<MLModelStatsSection data={stats} error={null} loading={false} onRefresh={onRefresh} />);

    fireEvent.click(screen.getByRole('button', { name: /refresh/i }));

    expect(onRefresh).toHaveBeenCalled();
  });

  it('shows the error rather than an empty table', () => {
    render(
      <MLModelStatsSection
        data={null}
        error="ML service unreachable"
        loading={false}
        onRefresh={vi.fn()}
      />,
    );

    expect(screen.getByText(/ML service unreachable/)).toBeInTheDocument();
  });

  it('will not refresh again while a refresh is in flight', () => {
    const onRefresh = vi.fn();
    render(<MLModelStatsSection data={null} error={null} loading onRefresh={onRefresh} />);

    const button = screen.getByRole('button', { name: /refreshing/i });
    expect(button).toBeDisabled();
  });
});
