import React from 'react';
import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import MLPredictionGraph from './MLPredictionGraph';

/** Minimal ResizeObserver so React Flow's guard passes; jsdom does not ship one. */
class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

describe('MLPredictionGraph', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('falls back to a notice when the browser has no ResizeObserver', () => {
    render(<MLPredictionGraph />);

    expect(
      screen.getByText(/available only in browsers that support ResizeObserver/i),
    ).toBeDefined();
  });

  it('renders every stage of the prediction flow', () => {
    vi.stubGlobal('ResizeObserver', ResizeObserverStub);

    render(<MLPredictionGraph />);

    const stages = [
      'Features at cutoff',
      'Batting model',
      'Bowling model',
      'Fielding model',
      'Innings model',
      'Per-player predictions',
      'Constraint reconciliation',
      'Team aggregates + Extras model',
      'Win model',
      'Combination meta model (optional)',
      'Feedback loop',
      'Team selection & scorecard',
      'Monte Carlo simulation',
      'Outcome distributions',
    ];
    stages.forEach((stage) => {
      expect(screen.getByText(stage)).toBeDefined();
    });
  });
});
