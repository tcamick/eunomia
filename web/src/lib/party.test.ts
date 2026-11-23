import { test } from 'node:test';
import assert from 'node:assert/strict';
import { partyCode, partyStyle, vacantStyle } from './party.ts';

test('every party, an unknown party and a vacant seat each have their own shape', () => {
  // Colour is never the only cue (web-conventions, party colour), so no two
  // of these may share a shape.
  const codes = ['D', 'R', 'I', 'O', ''];
  const shapes = [...codes.map((c) => partyStyle(c).shape), vacantStyle.shape];
  assert.equal(new Set(shapes).size, shapes.length, `shapes ${shapes.join(', ')}`);
  assert.notEqual(partyStyle('').token, vacantStyle.token, 'party unknown is not drawn in the vacant colour');
});

test('an unlisted code is drawn as another party', () => {
  assert.deepEqual(partyStyle('Z'), partyStyle('O'));
});

test('partyCode matches the server', () => {
  const cases: Record<string, [string | undefined, string]> = {
    democratic: ['Democratic', 'D'],
    republican: ['Republican', 'R'],
    independent: ['Independent', 'I'],
    another: ['Libertarian', 'O'],
    unknown: ['', ''],
    missing: [undefined, ''],
  };
  for (const [name, [party, want]] of Object.entries(cases)) {
    assert.equal(partyCode(party), want, name);
  }
});
