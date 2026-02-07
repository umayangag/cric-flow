import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { vi, describe, beforeEach, afterEach, it, expect } from 'vitest';
import React from 'react';
import OpsSuggestions from '../../src/components/OpsSuggestions';
import { fetchOpsSuggestions } from '../../src/api/client';

// Mock the API client
vi.mock('../../src/api/client', () => ({
  fetchOpsSuggestions: vi.fn(),
}));

describe('OpsSuggestions', () => {
  const origClipboard = global.navigator.clipboard;
  beforeEach(() => {
    const mockClipboard = {
      writeText: vi.fn().mockResolvedValue(undefined),
    } as unknown as Clipboard;
    // @ts-expect-error override for test
    global.navigator.clipboard = mockClipboard;
  });
  afterEach(() => {
    // @ts-expect-error restore clipboard
    global.navigator.clipboard = origClipboard as Clipboard;
    vi.clearAllMocks();
  });

  it('renders empty state when no suggestions', async () => {
    vi.mocked(fetchOpsSuggestions).mockResolvedValue([]);
    render(<OpsSuggestions />);
    // Wait for the effect to run and render "No suggestions"
    await waitFor(() => expect(screen.getByText(/No suggestions/i)).toBeInTheDocument());
  });

  it('copies commands when clicking Copy', async () => {
    const suggestions = [
      {
        title: 'DB not ready',
        description: 'Run migrations',
        command: 'make migrate && make cricsheet-import',
        priority: 'HIGH',
      },
    ];
    vi.mocked(fetchOpsSuggestions).mockResolvedValue(suggestions);

    render(<OpsSuggestions />);

    // Wait for suggestions to render
    await waitFor(() => expect(screen.getByText('DB not ready')).toBeInTheDocument());

    const btn = screen.getByRole('button', { name: /copy/i });
    fireEvent.click(btn);
    expect(global.navigator.clipboard.writeText).toHaveBeenCalledWith(
      'make migrate && make cricsheet-import',
    );
  });

  it('renders error message on API failure', async () => {
    vi.mocked(fetchOpsSuggestions).mockRejectedValue(new Error('API Error'));
    render(<OpsSuggestions />);
    await waitFor(() => expect(screen.getByText('Error: API Error')).toBeInTheDocument());
  });
});
