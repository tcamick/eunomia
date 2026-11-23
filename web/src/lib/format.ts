// Dates and times for display (docs/web-conventions.md, Dates and numbers).
//
// An instant (when a job ran, when a story was published) is shown in the
// viewer's local time. A calendar date (an election day, a term's start)
// arrives as YYYY-MM-DD and is never parsed with new Date(): west of UTC it
// would shift back a day. Calendar dates get their own function when the
// first one appears (E1a).

const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
  ['second', 1],
];

/** relativeTime says how long ago (or until) an instant is, from now: "3 hours ago". */
export function relativeTime(iso: string, now: Date, locale?: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return 'unknown';
  const seconds = Math.round((then - now.getTime()) / 1000);
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  for (const [unit, size] of units) {
    if (Math.abs(seconds) >= size || unit === 'second') {
      return rtf.format(Math.round(seconds / size), unit);
    }
  }
  return rtf.format(0, 'second');
}

/** localDateTime is an instant in full, in the viewer's time zone. */
export function localDateTime(iso: string, locale?: string, timeZone?: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return 'unknown';
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short', timeZone }).format(then);
}

/** duration is a job's run time in plain words: "850 ms", "4.2 s", "3 min". */
export function duration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return 'unknown';
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
  return `${Math.round(ms / 60_000)} min`;
}
