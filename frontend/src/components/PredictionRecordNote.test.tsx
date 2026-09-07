import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import PredictionRecordNote, {
  PREDICTION_NOT_RECORDED_TITLE,
  PREDICTION_RECORD_TITLE,
} from './PredictionRecordNote';

describe('PredictionRecordNote', () => {
  it('names the row a stored answer was filed under', () => {
    render(
      <PredictionRecordNote
        record={{ stored: true, id: '2b0f6c0e-1a4d-4e0a-9d7a-91f2b5c7a001' }}
      />,
    );

    const note = screen.getByTestId('prediction-record');
    expect(note).toHaveTextContent('recorded as 2b0f6c0e-1a4d-4e0a-9d7a-91f2b5c7a001');
    expect(note).toHaveAttribute('title', PREDICTION_RECORD_TITLE);
  });

  /**
   * §8.7 at the surface: an answer that was served but not filed is a hole in the track
   * record, and the person reading the number is the only one who can see it happen. The
   * reason comes off the wire; nothing here invents one.
   */
  it('shows the reason an answer was not recorded', () => {
    render(
      <PredictionRecordNote
        record={{ stored: false, reason: 'record prediction: connection refused' }}
      />,
    );

    const note = screen.getByTestId('prediction-record');
    expect(note).toHaveTextContent('not recorded: record prediction: connection refused');
    expect(note).toHaveAttribute('title', PREDICTION_NOT_RECORDED_TITLE);
  });

  it('still says the answer was not recorded when the store gave no reason', () => {
    render(<PredictionRecordNote record={{ stored: false }} />);

    expect(screen.getByTestId('prediction-record')).toHaveTextContent(
      'not recorded: the store gave no reason',
    );
  });
});
