import { render, screen, cleanup } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import React from 'react';
import ClubLineageLine from './ClubLineageLine';
import { readTeamLineage } from '../utils/opsStatusHelpers';

afterEach(cleanup);

describe('readTeamLineage', () => {
  it('reads the counts and the named renames off an /ops/status payload', () => {
    const lineage = readTeamLineage({
      db: {
        team_lineage: {
          status: 'incomplete',
          renames_configured: 10,
          linked: 9,
          unlinked: 1,
          unlinked_renames: ['Delhi Daredevils -> Delhi Capitals (male)'],
        },
      },
    });

    expect(lineage.status).toBe('incomplete');
    expect(lineage.renamesConfigured).toBe(10);
    expect(lineage.linked).toBe(9);
    expect(lineage.unlinkedRenames).toEqual(['Delhi Daredevils -> Delhi Capitals (male)']);
  });

  it('reads a payload with no lineage section as unknown rather than as zero linked', () => {
    const lineage = readTeamLineage({ db: {} });

    expect(lineage.status).toBe('unknown');
    expect(lineage.linked).toBeNull();
  });
});

describe('ClubLineageLine', () => {
  it('shows the count when every renamed club is linked', () => {
    render(
      <ClubLineageLine
        lineage={{ status: 'ok', renamesConfigured: 10, linked: 10, unlinkedRenames: [] }}
      />,
    );

    expect(screen.getByText('10 of 10 renamed clubs linked')).toBeTruthy();
  });

  it('calls out an archive whose links were never written', () => {
    render(
      <ClubLineageLine
        lineage={{
          status: 'incomplete',
          renamesConfigured: 10,
          linked: 9,
          unlinkedRenames: ['Delhi Daredevils -> Delhi Capitals (male)'],
        }}
      />,
    );

    expect(screen.getByText(/1 unlinked/)).toBeTruthy();
  });

  it('says so when the archive could not be read, rather than showing a zero', () => {
    render(
      <ClubLineageLine
        lineage={{ status: 'unknown', renamesConfigured: null, linked: null, unlinkedRenames: [] }}
      />,
    );

    expect(screen.getByText('not available')).toBeTruthy();
    expect(screen.queryByText(/renamed clubs linked/)).toBeNull();
  });
});
