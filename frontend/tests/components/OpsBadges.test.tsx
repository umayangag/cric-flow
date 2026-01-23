import { render, screen } from '@testing-library/react';
import React from 'react';
import OpsBadges from '../../src/components/OpsBadges';

describe('OpsBadges', () => {
  it('renders badges with states and last updated', () => {
    render(
      <OpsBadges
        services={{ api_health: true, api_readiness: false, ml_health: undefined }}
        timestamp={'2026-01-22T10:00:00Z'}
      />
    );

    expect(screen.getByText(/API: Healthy/i)).toBeInTheDocument();
    expect(screen.getByText(/DB Ready: Down/i)).toBeInTheDocument();
    // ML unknown shows as Unknown
    expect(screen.getByText(/ML: Unknown/i)).toBeInTheDocument();
    expect(screen.getByText(/Last updated:/i)).toBeInTheDocument();
  });
});
