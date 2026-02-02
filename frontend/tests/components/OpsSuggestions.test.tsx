import { fireEvent, render, screen } from '@testing-library/react';
import { vi } from 'vitest';
import React from 'react';
import OpsSuggestions from '../../src/components/OpsSuggestions';

describe('OpsSuggestions', () => {
  const origClipboard = global.navigator.clipboard;
  beforeEach(() => {
    // @ts-expect-error mock clipboard
    global.navigator.clipboard = {
      writeText: vi.fn().mockResolvedValue(undefined),
    } as any;
  });
  afterEach(() => {
    // @ts-expect-error restore clipboard
    global.navigator.clipboard = origClipboard as any;
  });

  it('renders empty state when no suggestions', () => {
    render(<OpsSuggestions suggestions={[]} />);
    expect(screen.getByText(/No suggestions/i)).toBeInTheDocument();
  });

  it('copies commands when clicking Copy', async () => {
    const suggestions = [
      {
        reason: 'DB not ready',
        commands: ['make migrate', 'make cricsheet-import'],
      },
    ];
    render(<OpsSuggestions suggestions={suggestions} />);
    const btn = screen.getByRole('button', { name: /copy/i });
    fireEvent.click(btn);
    expect(global.navigator.clipboard.writeText).toHaveBeenCalledWith(
      'make migrate && make cricsheet-import',
    );
  });
});
