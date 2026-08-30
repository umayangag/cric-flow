import { render, screen } from '@testing-library/react';
import React from 'react';
import RootErrorBoundary from './RootErrorBoundary';

const Boom: React.FC<{ message: string }> = ({ message }) => {
  throw new Error(message);
};

describe('RootErrorBoundary', () => {
  beforeEach(() => {
    // React logs the caught error; keep the test output readable.
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders children when nothing throws', () => {
    render(
      <RootErrorBoundary>
        <p>healthy</p>
      </RootErrorBoundary>,
    );

    expect(screen.getByText('healthy')).toBeInTheDocument();
  });

  it('shows the failure message instead of a blank page when a child throws', () => {
    render(
      <RootErrorBoundary>
        <Boom message="kaboom" />
      </RootErrorBoundary>,
    );

    expect(screen.getByText(/interface failed to render/i)).toBeInTheDocument();
    expect(screen.getByText('kaboom')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /reload/i })).toBeInTheDocument();
  });

  it('surfaces the dev:clean recovery command for a stale dependency cache', () => {
    render(
      <RootErrorBoundary>
        <Boom message="styled_default is not a function" />
      </RootErrorBoundary>,
    );

    expect(screen.getByText(/dev:clean/)).toBeInTheDocument();
  });
});
