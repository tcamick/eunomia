// Dates and times for display (docs/web-conventions.md, Dates and numbers).
//
// An instant (when a job ran, when a story was published) is shown in the
// viewer's local time. A calendar date (an election day, a term's start)
// arrives as YYYY-MM-DD and is never parsed with new Date() and formatted
// locally: west of UTC it would shift back a day. calendarDate reads it as
// UTC and formats it in UTC.

const reDate = /^(\d{4})-(\d{2})-(\d{2})$/;

/** calendarDate writes a YYYY-MM-DD date for people: "Jan 3, 2027". */
export function calendarDate(ymd: string | undefined, locale?: string): string {
  const m = reDate.exec(ymd ?? '');
  if (!m) return 'unknown';
  const t = Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(t);
}

/** year is the year of a YYYY-MM-DD date. */
export function year(ymd: string | undefined): string {
  return reDate.exec(ymd ?? '')?.[1] ?? '';
}

/** ordinal writes 12 as 12th. */
export function ordinal(n: number): string {
  const tens = n % 100;
  if (tens >= 11 && tens <= 13) return `${n}th`;
  return `${n}${({ 1: 'st', 2: 'nd', 3: 'rd' } as Record<number, string>)[n % 10] ?? 'th'}`;
}

/** seatName says which seat a member holds: "California's 12th district". */
export function seatName(m: { body: string; stateName: string; district?: number }): string {
  if (m.body === 'senate') return m.stateName;
  if (!m.district) return `${m.stateName} at large`;
  return `${m.stateName}'s ${ordinal(m.district)} district`;
}

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
