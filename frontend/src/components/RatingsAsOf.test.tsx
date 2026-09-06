import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import RatingsAsOf, { RATINGS_AS_OF_TITLE } from './RatingsAsOf';

describe('RatingsAsOf', () => {
  // One component says the date everywhere (P1-4), so this is the whole of what it says.
  it('renders the date and the run with the one-sentence tooltip', () => {
    render(
      <RatingsAsOf
        served={{ ratings_through: '2026-09-02', run_id: '20260906T083819Z-36689f80' }}
      />,
    );

    const asOf = screen.getByTestId('ratings-as-of');
    expect(asOf).toHaveTextContent('ratings as of 2026-09-02 · run 20260906T083819Z-36689f80');
    expect(asOf).toHaveAttribute('title', RATINGS_AS_OF_TITLE);
  });
});
