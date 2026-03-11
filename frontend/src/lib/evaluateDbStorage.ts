/** localStorage key for persisting the current evaluation job (survives refresh). */
export const EVAL_JOB_STORAGE_KEY = 'cric_info_eval_job';

export type StoredEvalJob = {
  job_id: string;
  match_id: number;
  format: string;
  team1: string;
  team2: string;
  started_at: string;
};

export function getStoredEvalJob(): StoredEvalJob | null {
  try {
    if (typeof localStorage?.getItem !== 'function') return null;
    const raw = localStorage.getItem(EVAL_JOB_STORAGE_KEY);
    if (!raw) return null;
    const data = JSON.parse(raw) as StoredEvalJob;
    return data?.job_id ? data : null;
  } catch {
    return null;
  }
}

export function setStoredEvalJob(job: StoredEvalJob): void {
  try {
    localStorage.setItem(EVAL_JOB_STORAGE_KEY, JSON.stringify(job));
  } catch {
    /* ignore (e.g. private mode) */
  }
}

export function clearStoredEvalJob(): void {
  try {
    localStorage.removeItem(EVAL_JOB_STORAGE_KEY);
  } catch {
    /* ignore */
  }
}
