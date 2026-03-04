import { useMemo, useState, useEffect, useRef } from 'react';
import { api } from '../api';
import type { PredictTeamSelectionResponse } from '../types';

const MAX_FUTURE_DAYS = 14;

function formatDateForInput(d: Date): string {
  return d.toISOString().slice(0, 10);
}

export function useUpcomingMatch() {
  const [format, setFormat] = useState('');
  const [team1, setTeam1] = useState('');
  const [team2, setTeam2] = useState('');
  const [venue, setVenue] = useState('');
  const [matchDate, setMatchDate] = useState('');
  const [predictionModel, setPredictionModel] = useState<'format' | 'unified'>('format');
  const [runSimulation, setRunSimulation] = useState(false);
  const [availableFormats, setAvailableFormats] = useState<string[]>([]);
  const [availableTeam1s, setAvailableTeam1s] = useState<string[]>([]);
  const [availableTeam2s, setAvailableTeam2s] = useState<string[]>([]);
  const [venueOptions, setVenueOptions] = useState<string[]>([]);
  const [venueLoading, setVenueLoading] = useState(false);
  const venueSearchRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<PredictTeamSelectionResponse | null>(null);

  const { minDate, maxDate } = useMemo(() => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const max = new Date(today);
    max.setDate(max.getDate() + MAX_FUTURE_DAYS);
    return { minDate: formatDateForInput(today), maxDate: formatDateForInput(max) };
  }, []);

  useEffect(() => {
    let active = true;
    api.getFormats().then((f) => { if (active) setAvailableFormats(f); }).catch((e) => {
      if (active) setError(`Failed to load formats: ${e instanceof Error ? e.message : String(e)}`);
    });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (!format) { setAvailableTeam1s([]); setTeam1(''); return; }
    let active = true;
    api.getTeamsByFormat(format).then((teams) => {
      if (active) { setAvailableTeam1s(teams); setTeam1((prev) => (prev && !teams.includes(prev) ? '' : prev)); }
    }).catch((e) => { if (active) setError(`Failed to load teams: ${e instanceof Error ? e.message : String(e)}`); });
    return () => { active = false; };
  }, [format]);

  useEffect(() => {
    if (!format || !team1) { setAvailableTeam2s([]); setTeam2(''); return; }
    let active = true;
    api.getOpponents(format, team1).then((opps) => {
      if (active) { setAvailableTeam2s(opps); setTeam2((prev) => (prev && !opps.includes(prev) ? '' : prev)); }
    }).catch((e) => { if (active) setError(`Failed to load opponents: ${e instanceof Error ? e.message : String(e)}`); });
    return () => { active = false; };
  }, [format, team1]);

  const fetchVenueOptions = (query: string) => {
    const trimmed = query.trim();
    if (trimmed.length < 3) { setVenueOptions([]); return; }
    setVenueLoading(true);
    api.searchVenues(trimmed).then((list) => setVenueOptions(list)).catch(() => setVenueOptions([])).finally(() => setVenueLoading(false));
  };

  const handleVenueInputChange = (_: React.SyntheticEvent, value: string) => {
    setVenue(value);
    if (venueSearchRef.current) { clearTimeout(venueSearchRef.current); venueSearchRef.current = null; }
    if (value.trim().length < 3) { setVenueOptions([]); return; }
    venueSearchRef.current = setTimeout(() => fetchVenueOptions(value), 300);
  };

  useEffect(() => { return () => { if (venueSearchRef.current) clearTimeout(venueSearchRef.current); }; }, []);

  const dateError = useMemo(() => {
    if (!matchDate) return 'Match date is required';
    const [y, m, d] = matchDate.split('-').map(Number);
    const selectedLocal = new Date(y, m - 1, d);
    const today = new Date(); today.setHours(0, 0, 0, 0);
    const max = new Date(today); max.setDate(max.getDate() + MAX_FUTURE_DAYS);
    if (selectedLocal < today) return 'Date must be today or in the future';
    if (selectedLocal > max) return `Date must be within ${MAX_FUTURE_DAYS} days from today`;
    return null;
  }, [matchDate]);

  const canPredict = useMemo(() => !!format && !!team1 && !!team2 && !!matchDate && !dateError && !loading, [format, team1, team2, matchDate, dateError, loading]);

  const handlePredict = async () => {
    if (!canPredict) return;
    setError(null); setResult(null); setLoading(true);
    try {
      const res = await api.predictTeamSelection({
        format: format.trim(), team1: team1.trim(), team2: team2.trim(),
        venue: venue.trim() || undefined, match_date: matchDate,
        use_unified_model: predictionModel === 'unified', simulate: runSimulation,
      });
      setResult(res);
    } catch (e) { setError(e instanceof Error ? e.message : String(e)); }
    finally { setLoading(false); }
  };

  return {
    format, setFormat, team1, setTeam1, team2, setTeam2, venue, setVenue,
    matchDate, setMatchDate, predictionModel, setPredictionModel,
    runSimulation, setRunSimulation, availableFormats, availableTeam1s,
    availableTeam2s, venueOptions, venueLoading, handleVenueInputChange,
    loading, error, result, minDate, maxDate, dateError, canPredict,
    handlePredict, maxFutureDays: MAX_FUTURE_DAYS,
  };
}
