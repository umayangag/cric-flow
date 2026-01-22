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

    expect(screen.getByText(/API: OK/i)).toBeInTheDocument();
    expect(screen.getByText(/DB Ready: DOWN/i)).toBeInTheDocument();
    // ML unknown shown as em dash
    expect(screen.getByText(/ML: —/i)).toBeInTheDocument();
    expect(screen.getByText(/Last updated:/i)).toBeInTheDocument();
  });
});
