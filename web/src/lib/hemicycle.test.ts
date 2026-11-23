import { test } from 'node:test';
import assert from 'node:assert/strict';
import { hemicycle, rowsFor } from './hemicycle.ts';
import { partyLabel, partyStyle, seatingOrder } from './party.ts';

test('hemicycle places exactly n seats', () => {
  for (const n of [1, 2, 7, 100, 435, 441]) {
    assert.equal(hemicycle(n).seats.length, n, `${n} seats`);
  }
  assert.equal(hemicycle(0).seats.length, 0);
});

test('hemicycle numbers seats from left to right', () => {
  const { seats } = hemicycle(100);
  assert.ok(seats[0]!.x < seats[99]!.x, 'the first seat is on the left, the last on the right');
  assert.ok(seats[0]!.x < 100 && seats[99]!.x > 100);
});

test('hemicycle keeps seats inside the drawing and apart', () => {
  const l = hemicycle(435);
  for (const s of l.seats) {
    assert.ok(s.x - l.r >= 0 && s.x + l.r <= l.width, 'inside horizontally');
    assert.ok(s.y - l.r >= 0 && s.y + l.r <= l.height, 'inside vertically');
  }
  // Seats in the same ring do not overlap: check neighbours by distance.
  let tooClose = 0;
  for (let i = 0; i < l.seats.length; i += 7) {
    for (let k = i + 1; k < l.seats.length; k++) {
      const dx = l.seats[i]!.x - l.seats[k]!.x;
      const dy = l.seats[i]!.y - l.seats[k]!.y;
      if (Math.hypot(dx, dy) < 2 * l.r) tooClose++;
    }
  }
  assert.equal(tooClose, 0, 'no two seats overlap');
});

test('rowsFor grows with the chamber', () => {
  assert.ok(rowsFor(100) < rowsFor(435));
});

test('seatingOrder draws Democrats left and Republicans right, independents beside their caucus', () => {
  const order = seatingOrder([
    { party: 'Republican', code: 'R', seats: 53 },
    { party: 'Independent', caucus: 'Democratic', code: 'I', seats: 2 },
    { party: 'Democratic', code: 'D', seats: 45 },
    { party: 'Independent', caucus: 'Republican', code: 'I', seats: 1 },
    { party: 'Libertarian', code: 'O', seats: 1 },
  ]);
  assert.deepEqual(
    order.map((g) => `${g.party}/${g.caucus ?? ''}`),
    ['Democratic/', 'Independent/Democratic', 'Libertarian/', 'Independent/Republican', 'Republican/'],
  );
});

test('every party has a shape as well as a colour', () => {
  const shapes = new Set(['D', 'R', 'I', 'O', ''].map((c) => partyStyle(c).shape));
  assert.equal(shapes.size, 5, 'five distinct shapes, so colour is never the only signal');
  assert.equal(partyStyle('X').shape, partyStyle('O').shape, 'an unknown code is drawn as other');
});

test('partyLabel names the caucus only when it differs', () => {
  assert.equal(partyLabel('Independent', 'Democratic'), 'Independent, caucuses with Democrats');
  assert.equal(partyLabel('Republican', ''), 'Republican');
  assert.equal(partyLabel('', ''), 'Party unknown');
});
