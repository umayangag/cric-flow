import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { vi, describe, beforeEach, afterEach, it, expect } from 'vitest';
import React from 'react';
import OpsSuggestions from '../../src/components/OpsSuggestions';
import { api } from '../../src/api';

// Mock the API client
vi.mock('../../src/api', () => ({
  api: {
    opsSuggestions: vi.fn(),
  },
}));

describe('OpsSuggestions', () => {
  const mockWriteText = vi.fn().mockResolvedValue(undefined);
  const mockClipboard = { writeText: mockWriteText };

  beforeEach(() => {
    mockWriteText.mockClear();
    Object.defineProperty(global.navigator, 'clipboard', {
      value: mockClipboard,
      writable: true,
      configurable: true,
    });
  });
  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders empty state when no suggestions', async () => {
    vi.mocked(api.opsSuggestions).mockResolvedValue([]);
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
    vi.mocked(api.opsSuggestions).mockResolvedValue(suggestions);

    render(<OpsSuggestions />);

    // Wait for suggestions to render
    await waitFor(() => expect(screen.getByText('DB not ready')).toBeInTheDocument());

    const btn = screen.getByRole('button', { name: /copy/i });
    await act(async () => {
      fireEvent.click(btn);
    });
    expect(mockWriteText).toHaveBeenCalledWith('make migrate && make cricsheet-import');
    await waitFor(() => expect(screen.getByText('Copied!')).toBeInTheDocument());
  });

  it('renders error message on API failure', async () => {
    vi.mocked(api.opsSuggestions).mockRejectedValue(new Error('API Error'));
    render(<OpsSuggestions />);
    await waitFor(() => expect(screen.getByText('Error: API Error')).toBeInTheDocument());
  });
});
