import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Delegation, StateSummary } from './api.ts';
import { fillFor, legend, shortLabel, spoken } from './mapmode.ts';

const D = { party: 'Democratic', code: 'D' };
const R = { party: 'Republican', code: 'R' };
const I = { party: 'Independent', caucus: 'Democratic', code: 'I' };

const del = (seats: number, vacant: number, lead: string, parties: Delegation['parties']): Delegation => ({
  seats,
  vacant,
  lead,
  parties,
});

const state = (code: string, name: string, senate: Delegation, house: Delegation, voting = true): StateSummary => ({
  code,
  name,
  kind: voting ? 'state' : 'district',
  voting,
  senate,
  house,
});

const cases = {
  'all one party': del(2, 0, 'D', [{ ...D, seats: 2 }]),
  'a split delegation': del(2, 0, 'even', [
    { ...D, seats: 1 },
    { ...R, seats: 1 },
  ]),
  'an Independent who caucuses with the Democrats': del(2, 0, 'even', [
    { ...I, seats: 1 },
    { ...R, seats: 1 },
  ]),
  'one seat vacant': del(2, 1, 'R', [{ ...R, seats: 1 }]),
  'both vacant': del(2, 2, 'vacant', []),
  'an at-large seat': del(1, 0, 'R', [{ ...R, seats: 1 }]),
  'no seats': del(0, 0, 'none', []),
  'party unknown': del(1, 0, '', [{ party: '', code: '', seats: 1 }]),
};

test('fillFor paints a lead party, stripes for a tie, a hatch for vacant, nothing for none', () => {
  assert.deepEqual(fillFor(cases['all one party']), { kind: 'party', code: 'D' });
  assert.deepEqual(fillFor(cases['a split delegation']), { kind: 'even', codes: ['D', 'R'] });
  assert.deepEqual(fillFor(cases['an Independent who caucuses with the Democrats']), { kind: 'even', codes: ['I', 'R'] });
  assert.deepEqual(fillFor(cases['one seat vacant']), { kind: 'party', code: 'R' });
  assert.deepEqual(fillFor(cases['both vacant']), { kind: 'vacant' });
  assert.deepEqual(fillFor(cases['an at-large seat']), { kind: 'party', code: 'R' });
  assert.deepEqual(fillFor(cases['no seats']), { kind: 'none' });
  assert.deepEqual(fillFor(cases['party unknown']), { kind: 'party', code: '' }, 'unknown, never a guessed party');
});

test('shortLabel says the seats in letters, so colour is never alone', () => {
  const want: Record<keyof typeof cases, string> = {
    'all one party': '2 D',
    'a split delegation': '1 D · 1 R',
    'an Independent who caucuses with the Democrats': '1 I · 1 R',
    'one seat vacant': '1 R · 1 vacant',
    'both vacant': '2 vacant',
    'an at-large seat': '1 R',
    'no seats': '',
    'party unknown': '1 ?',
  };
  for (const [name, d] of Object.entries(cases)) {
    assert.equal(shortLabel(d), want[name as keyof typeof cases], name);
  }
});

test('spoken names the state, the chamber and every party in words', () => {
  const oh = state('OH', 'Ohio', cases['a split delegation'], cases['one seat vacant']);
  assert.equal(spoken(oh, 'senate'), 'Ohio: senators 1 Democratic, 1 Republican');
  assert.equal(spoken(oh, 'house'), 'Ohio: House 1 Republican, 1 vacant');
  const me = state('ME', 'Maine', cases['an Independent who caucuses with the Democrats'], cases['all one party']);
  assert.equal(spoken(me, 'senate'), 'Maine: senators 1 Independent (caucuses with Democrats), 1 Republican');
  const dc = state('DC', 'District of Columbia', cases['no seats'], del(1, 0, 'D', [{ ...D, seats: 1 }]), false);
  assert.equal(spoken(dc, 'senate'), 'District of Columbia: no senators');
  assert.equal(spoken(dc, 'house'), 'District of Columbia: non-voting delegate 1 Democratic');
});

test('legend lists only what the map shows, parties first, then ties, then vacant', () => {
  const ss = [
    state('PA', 'Pennsylvania', cases['a split delegation'], cases['all one party']),
    state('OH', 'Ohio', cases['both vacant'], cases['an at-large seat']),
    state('DE', 'Delaware', cases['all one party'], cases['all one party']),
  ];
  assert.deepEqual(
    legend(ss, 'senate').map((e) => e.key),
    ['party:D', 'even:D-R', 'vacant'],
  );
  assert.deepEqual(
    legend(ss, 'house').map((e) => e.key),
    ['party:D', 'party:R'],
  );
});
