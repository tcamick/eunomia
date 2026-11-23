import { test } from 'node:test';
import assert from 'node:assert/strict';
import { httpsOnly } from './url.ts';

test('httpsOnly keeps https and drops everything else', () => {
  const cases: Record<string, [string | undefined, string]> = {
    https: ['https://www.senate.gov/', 'https://www.senate.gov/'],
    http: ['http://example.house.gov/', ''],
    script: ['javascript:alert(1)', ''],
    relative: ['/people/A000001', ''],
    empty: ['', ''],
    missing: [undefined, ''],
  };
  for (const [name, [input, want]] of Object.entries(cases)) {
    assert.equal(httpsOnly(input), want, name);
  }
});
