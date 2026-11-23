import { test } from 'node:test';
import assert from 'node:assert/strict';
import { calendarDate, duration, localDate, todayUTC, localDateTime, ordinal, relativeTime, seatName, year } from './format.ts';

test('calendarDate never shifts a date west of UTC', () => {
  // Formatting with UTC: the 3rd stays the 3rd whatever the viewer's zone.
  assert.equal(calendarDate('2027-01-03', 'en-US'), 'Jan 3, 2027');
  assert.equal(calendarDate('2026-11-03', 'en-US'), 'Nov 3, 2026');
  assert.equal(calendarDate('nonsense', 'en-US'), 'unknown');
  assert.equal(calendarDate(undefined, 'en-US'), 'unknown');
  assert.equal(year('2027-01-03'), '2027');
});

test('ordinal and seatName', () => {
  assert.deepEqual([1, 2, 3, 4, 11, 12, 13, 21, 22, 52].map(ordinal), ['1st', '2nd', '3rd', '4th', '11th', '12th', '13th', '21st', '22nd', '52nd']);
  assert.equal(seatName({ body: 'house', stateName: 'California', district: 12 }), "California's 12th district");
  assert.equal(seatName({ body: 'house', stateName: 'Wyoming', district: 0 }), 'Wyoming at large');
  assert.equal(seatName({ body: 'senate', stateName: 'Vermont' }), 'Vermont');
});

const now = new Date('2026-11-22T12:00:00Z');

test('relativeTime picks the largest whole unit', () => {
  assert.equal(relativeTime('2026-11-22T09:00:00Z', now, 'en'), '3 hours ago');
  assert.equal(relativeTime('2026-11-21T12:00:00Z', now, 'en'), 'yesterday');
  assert.equal(relativeTime('2026-11-22T11:59:30Z', now, 'en'), '30 seconds ago');
  assert.equal(relativeTime('2026-11-22T12:00:00Z', now, 'en'), 'now');
});

test('relativeTime says unknown for a value it cannot read', () => {
  assert.equal(relativeTime('not a time', now, 'en'), 'unknown');
});

test('localDateTime shows the instant in the given time zone', () => {
  // 02:00 UTC on the 22nd is still the 21st in California.
  assert.match(localDateTime('2026-11-22T02:00:00Z', 'en-US', 'America/Los_Angeles'), /Nov 21, 2026/);
  assert.match(localDateTime('2026-11-22T02:00:00Z', 'en-US', 'UTC'), /Nov 22, 2026/);
});

test('duration reads as plain words', () => {
  assert.equal(duration(850), '850 ms');
  assert.equal(duration(4200), '4.2 s');
  assert.equal(duration(180_000), '3 min');
  assert.equal(duration(-1), 'unknown');
});

test('localDate gives the day of an instant in the given zone', () => {
  // 03:00 UTC on Nov 23 is still Nov 22 in Los Angeles.
  assert.equal(localDate('2026-11-23T03:00:00Z', 'en-US', 'America/Los_Angeles'), 'Nov 22, 2026');
  assert.equal(localDate('2026-11-23T03:00:00Z', 'en-US', 'UTC'), 'Nov 23, 2026');
  assert.equal(localDate('not a time'), 'unknown');
  assert.equal(localDate(undefined), 'unknown');
});

test('todayUTC is the UTC calendar date', () => {
  assert.equal(todayUTC(new Date('2026-11-23T03:00:00Z')), '2026-11-23');
  assert.equal(todayUTC(new Date('2026-11-22T23:59:59-08:00')), '2026-11-23');
});
