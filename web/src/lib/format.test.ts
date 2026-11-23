import { test } from 'node:test';
import assert from 'node:assert/strict';
import { duration, localDateTime, relativeTime } from './format.ts';

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
