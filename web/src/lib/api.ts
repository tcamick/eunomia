// The JSON API's shapes and a small fetch helper. Every call is same-origin.

export interface JobStatus {
  name: string;
  lastRun: string;
  lastOk?: string;
  ok: boolean;
  durationMs: number;
  message?: string;
}

export interface Status {
  version: string;
  now: string;
  jobs: JobStatus[];
  jobsError?: string;
}

/** getJSON fetches a path under /api and throws with the status on failure. */
export async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const resp = await fetch(path, { headers: { Accept: 'application/json' }, signal });
  if (!resp.ok) throw new Error(`${path} answered ${resp.status}`);
  return (await resp.json()) as T;
}
