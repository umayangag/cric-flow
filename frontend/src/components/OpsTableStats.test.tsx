import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import OpsTableStats from './OpsTableStats';
import type { TableStat } from '../types';

describe('OpsTableStats', () => {
  it('returns null when stats is undefined', () => {
    const { container } = render(<OpsTableStats />);
    expect(container.firstChild).toBeNull();
  });

  it('returns null when stats is empty array', () => {
    const { container } = render(<OpsTableStats stats={[]} />);
    expect(container.firstChild).toBeNull();
  });

  it('renders table with table name, row count and last record', () => {
    const stats: TableStat[] = [
      { table_name: 'matches', row_count: 1000, last_record: '2024-01-15' },
      { table_name: 'players', row_count: 500, last_record: '2024-01-14' },
    ];
    render(<OpsTableStats stats={stats} />);
    expect(screen.getByRole('table', { name: /database table statistics/i })).toBeInTheDocument();
    expect(screen.getByText('matches')).toBeInTheDocument();
    expect(screen.getByText('players')).toBeInTheDocument();
    expect(screen.getByText('1,000')).toBeInTheDocument();
    expect(screen.getByText('500')).toBeInTheDocument();
    expect(screen.getByText('2024-01-15')).toBeInTheDocument();
    expect(screen.getByText('2024-01-14')).toBeInTheDocument();
  });

  it('renders dash when last_record is missing', () => {
    const stats: TableStat[] = [{ table_name: 'empty_table', row_count: 0 }];
    render(<OpsTableStats stats={stats} />);
    expect(screen.getByText('empty_table')).toBeInTheDocument();
    expect(screen.getByText('-')).toBeInTheDocument();
  });

  it('applies error styling for rows with zero row_count', () => {
    const stats: TableStat[] = [
      { table_name: 'zero_rows', row_count: 0 },
      { table_name: 'has_rows', row_count: 10, last_record: '2024-01-01' },
    ];
    const { container } = render(<OpsTableStats stats={stats} />);
    const rows = container.querySelectorAll('tbody tr');
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveAttribute('class', expect.stringContaining('MuiTableRow'));
  });
});
