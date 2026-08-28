import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import ErrorNotice from './ErrorNotice';
import { ApiError } from '../../lib/apiError';

describe('ErrorNotice', () => {
  it('renders nothing when there is no error', () => {
    const { container } = render(<ErrorNotice error={null} />);
    expect(container).toBeEmptyDOMElement();
  });

  /**
   * The plan's acceptance for W1-2, asserted rather than assumed: every backend error
   * carrying a hint shows that hint. It was being written by the backend and dropped
   * by the client, so nothing downstream could have shown it.
   */
  it('shows the hint the backend sent, and what is available', () => {
    render(
      <ErrorNotice
        error={
          new ApiError('Model for format T20I not loaded', {
            status: 404,
            code: 'MODEL_NOT_LOADED',
            hint: 'Train artifacts for this format and place them under the models directory.',
            available: ['ODI', 'TEST'],
          })
        }
      />,
    );

    expect(screen.getByText('Model for format T20I not loaded')).toBeInTheDocument();
    expect(screen.getByText(/Train artifacts for this format/)).toBeInTheDocument();
    expect(screen.getByText(/ODI, TEST/)).toBeInTheDocument();
    expect(screen.getByText('MODEL_NOT_LOADED')).toBeInTheDocument();
  });

  it('adds the remedy for a code it knows, and does not repeat the hint', () => {
    render(<ErrorNotice error={new ApiError('nope', { code: 'CONTRIBUTIONS_CSV_MISSING' })} />);
    expect(screen.getByText(/Evaluate → Export contributions/)).toBeInTheDocument();
  });

  it('accepts a plain string, for surfaces not yet on the structured error', () => {
    render(<ErrorNotice error="something broke" />);
    expect(screen.getByText('something broke')).toBeInTheDocument();
  });
});
