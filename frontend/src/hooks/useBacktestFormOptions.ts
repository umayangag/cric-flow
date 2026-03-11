import { useEffect, useState } from 'react';
import { api } from '../api';

export interface UseBacktestFormOptionsReturn {
  format: string;
  setFormat: (v: string) => void;
  team1: string;
  setTeam1: (v: string) => void;
  team2: string;
  setTeam2: (v: string) => void;
  availableFormats: string[];
  availableTeam1s: string[];
  availableTeam2s: string[];
  optionsError: string | null;
  setOptionsError: (v: string | null) => void;
}

/**
 * Manages format/team1/team2 form state and their option lists (formats, teams by format, opponents).
 * Loads options when format or team1 changes. Exposes setOptionsError so the parent can display load errors.
 */
export function useBacktestFormOptions(): UseBacktestFormOptionsReturn {
  const [format, setFormat] = useState<string>('');
  const [team1, setTeam1] = useState<string>('');
  const [team2, setTeam2] = useState<string>('');
  const [availableFormats, setAvailableFormats] = useState<string[]>([]);
  const [availableTeam1s, setAvailableTeam1s] = useState<string[]>([]);
  const [availableTeam2s, setAvailableTeam2s] = useState<string[]>([]);
  const [optionsError, setOptionsError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    const fetchData = async () => {
      try {
        const f = await api.getFormats();
        if (active) setAvailableFormats(f);
      } catch (e) {
        if (active) {
          setOptionsError(
            `Failed to load form options: ${e instanceof Error ? e.message : String(e)}`,
          );
        }
      }
    };
    fetchData();
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (!format) {
      setAvailableTeam1s([]);
      setTeam1('');
      return;
    }
    let active = true;
    api
      .getTeamsByFormat(format)
      .then((teams) => {
        if (active) {
          setAvailableTeam1s(teams);
          setTeam1((current) => (current && !teams.includes(current) ? '' : current));
        }
      })
      .catch((err) => {
        if (active) setOptionsError(`Failed to fetch teams: ${err.message}`);
      });
    return () => {
      active = false;
    };
  }, [format]);

  useEffect(() => {
    if (!format || !team1) {
      setAvailableTeam2s([]);
      setTeam2('');
      return;
    }
    let active = true;
    api
      .getOpponents(format, team1)
      .then((opps) => {
        if (active) {
          setAvailableTeam2s(opps);
          setTeam2((current) => (current && !opps.includes(current) ? '' : current));
        }
      })
      .catch((err) => {
        if (active) setOptionsError(`Failed to fetch opponents: ${err.message}`);
      });
    return () => {
      active = false;
    };
  }, [format, team1]);

  return {
    format,
    setFormat,
    team1,
    setTeam1,
    team2,
    setTeam2,
    availableFormats,
    availableTeam1s,
    availableTeam2s,
    optionsError,
    setOptionsError,
  };
}
