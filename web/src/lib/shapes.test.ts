import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import type { DistrictMap } from './geo-types.ts';
import { states, viewBox } from './geo/states.ts';

// The generated modules must say what their manifest says; the Go test
// (internal/geo, shapes_test.go) holds the manifest to the apportionment.
const manifest = JSON.parse(readFileSync(new URL('./geo/manifest.json', import.meta.url), 'utf8')) as {
  congress: number;
  states: string[];
  districts: Record<string, number[]>;
};

test('the map has a drawable shape for every state the manifest lists', () => {
  assert.equal(viewBox, '0 0 975 610');
  assert.deepEqual(
    states.map((s) => s.code),
    manifest.states,
  );
  for (const s of states) {
    assert.match(s.d, /^M[\d.-]/, `${s.code} has path data`);
    assert.ok(s.name.length > 0, `${s.code} has a name`);
  }
});

test("each state's district module has the manifest's districts, drawable, of its Congress", async () => {
  for (const [code, want] of Object.entries(manifest.districts)) {
    const map = ((await import(`./geo/districts/${code}.ts`)) as { default: DistrictMap }).default;
    assert.equal(map.state, code, `${code}'s module is ${code}'s`);
    assert.equal(map.congress, manifest.congress, `${code}'s lines are the ${manifest.congress}th Congress's`);
    assert.match(map.viewBox, /^0 0 600 \d+$/, `${code}'s frame`);
    assert.deepEqual(
      map.districts.map((d) => d.n),
      want,
      `${code}'s districts`,
    );
    for (const d of map.districts) assert.match(d.d, /^M[\d.-]/, `${code} district ${d.n} has path data`);
  }
});
