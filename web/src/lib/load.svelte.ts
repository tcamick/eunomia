// Loads one API response into reactive state, cancelling the previous
// request when the page asks for another.

import { getJSON, NotFound } from './api';

export class Load<T> {
  data = $state<T | null>(null);
  error = $state('');
  missing = $state(false);
  #ctrl: AbortController | null = null;

  /** get fetches path; the previous request, if still running, is cancelled. */
  get(path: string): void {
    this.#ctrl?.abort();
    const ctrl = new AbortController();
    this.#ctrl = ctrl;
    this.error = '';
    this.missing = false;
    getJSON<T>(path, ctrl.signal)
      .then((d) => {
        if (!ctrl.signal.aborted) this.data = d;
      })
      .catch((e: unknown) => {
        if (ctrl.signal.aborted) return;
        if (e instanceof NotFound) this.missing = true;
        else this.error = e instanceof Error ? e.message : String(e);
      });
  }

  /** stop cancels a running request; call it when the page goes away. */
  stop(): void {
    this.#ctrl?.abort();
  }
}
