import { useCallback, useEffect, useMemo, useState } from 'react';
import { api } from '../api';
import { useAsync } from './useAsync';
import { useVenueSearch } from './useVenueSearch';
import type { ApiError } from '../lib/apiError';
import type { PredictTeamSelectionResponse } from '../types';

const MAX_FUTURE_DAYS = 14;

function formatDateForInput(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/** Midnight today and the last date a prediction is offered for, as input values. */
function dateBounds(): { minDate: string; maxDate: string; min: Date; max: Date } {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const max = new Date(today);
  max.setDate(max.getDate() + MAX_FUTURE_DAYS);
  return {
    minDate: formatDateForInput(today),
    maxDate: formatDateForInput(max),
    min: today,
    max,
  };
}

/**
 * The Upcoming Match tab's state.
 *
 * It held 16 `useState` calls, five of which were three near-identical
 * fetch-and-cascade blocks (formats → teams → opponents) with their own `active`
 * flags, plus a hand-rolled debounce for venue search. The cascades are now three
 * {@link useAsync} calls whose effects say only what they depend on, and the debounce
 * moved to {@link useVenueSearch} where it can also discard a stale response — typing
 * "Lord's" used to let the answer for "Lor" land on top of it.
 *
 * What stays `useState` is the form: six fields the user types into, which are
 * component state and not an async lifecycle pretending to be one.
 */
export function useUpcomingMatch() {
  const [format, setFormat] = useState('');
  const [team1, setTeam1] = useState('');
  const [team2, setTeam2] = useState('');
  const [venue, setVenue] = useState('');
  const [matchDate, setMatchDate] = useState('');

  const formats = useAsync(api.getFormats, {
    runOnMount: [],
    errorMessage: 'Failed to load formats',
  });
  const teams = useAsync((fmt: string) => api.getTeamsByFormat(fmt), {
    errorMessage: 'Failed to load teams',
  });
  const opponents = useAsync((fmt: string, side: string) => api.getOpponents(fmt, side), {
    errorMessage: 'Failed to load opponents',
  });
  const prediction = useAsync(api.predictTeamSelection, {
    errorMessage: 'The prediction failed',
  });
  const venues = useVenueSearch();
  // The pipeline state, so the tab can say what a prediction will be missing before it
  // is run rather than after it has quietly answered on zeros (W4-2).
  const opsStatus = useAsync(api.opsStatus, { runOnMount: [] });

  const { run: loadTeams, reset: resetTeams } = teams;
  const { run: loadOpponents, reset: resetOpponents } = opponents;

  // Teams depend on the format; opponents on the format and team 1. Each cascade
  // clears the selection below it when the list it came from can no longer contain it.
  useEffect(() => {
    if (!format) {
      resetTeams();
      setTeam1('');
      return;
    }
    void loadTeams(format).then((list) => {
      if (list) setTeam1((prev) => (prev && !list.includes(prev) ? '' : prev));
    });
  }, [format, loadTeams, resetTeams]);

  useEffect(() => {
    if (!format || !team1) {
      resetOpponents();
      setTeam2('');
      return;
    }
    void loadOpponents(format, team1).then((list) => {
      if (list) setTeam2((prev) => (prev && !list.includes(prev) ? '' : prev));
    });
  }, [format, team1, loadOpponents, resetOpponents]);

  const { minDate, maxDate } = useMemo(dateBounds, []);

  const dateError = useMemo(() => {
    if (!matchDate) return 'Match date is required';
    const [y, m, d] = matchDate.split('-').map(Number);
    const selectedLocal = new Date(y, m - 1, d);
    const { min, max } = dateBounds();
    if (selectedLocal < min) return 'Date must be today or in the future';
    if (selectedLocal > max) return `Date must be within ${MAX_FUTURE_DAYS} days from today`;
    return null;
  }, [matchDate]);

  const canPredict =
    Boolean(format && team1 && team2 && matchDate) && !dateError && !prediction.loading;

  const handleVenueInputChange = useCallback(
    (_: React.SyntheticEvent, value: string) => {
      setVenue(value);
      venues.search(value);
    },
    [venues],
  );

  const { run: predict } = prediction;
  const handlePredict = useCallback(async () => {
    if (!canPredict) return;
    await predict({
      format: format.trim(),
      team1: team1.trim(),
      team2: team2.trim(),
      venue: venue.trim() || undefined,
      match_date: matchDate,
    });
  }, [canPredict, predict, format, team1, team2, venue, matchDate]);

  // One error at a time, newest first: a failed prediction is what the user just did,
  // and an option list that failed to load is visible as an empty picker anyway.
  const error: ApiError | null =
    prediction.error ?? opponents.error ?? teams.error ?? formats.error;

  return {
    format,
    setFormat,
    team1,
    setTeam1,
    team2,
    setTeam2,
    venue,
    setVenue,
    matchDate,
    setMatchDate,
    availableFormats: formats.data ?? [],
    availableTeam1s: teams.data ?? [],
    availableTeam2s: opponents.data ?? [],
    venueOptions: venues.options,
    venueLoading: venues.loading,
    handleVenueInputChange,
    loading: prediction.loading,
    error,
    result: prediction.data as PredictTeamSelectionResponse | null,
    minDate,
    maxDate,
    dateError,
    canPredict,
    handlePredict,
    maxFutureDays: MAX_FUTURE_DAYS,
    opsStatus: opsStatus.data,
  };
}
