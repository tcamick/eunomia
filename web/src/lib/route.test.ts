import { test } from 'node:test';
import assert from 'node:assert/strict';
import { match } from './route.ts';

test('match finds each page', () => {
  assert.equal(match('/').name, 'congress');
  assert.equal(match('/members').name, 'members');
  assert.equal(match('/members/').name, 'members', 'a trailing slash is the same page');
  assert.deepEqual(match('/people/S000033').params, { id: 'S000033' });
  assert.deepEqual(match('/committees/HSED14').params, { id: 'HSED14' });
  assert.equal(match('/search', '?q=sanders').query.get('q'), 'sanders');
});

test('match refuses malformed ids', () => {
  assert.equal(match('/people/../etc').name, 'notfound');
  assert.equal(match('/people/s000033').name, 'notfound');
  assert.equal(match('/committees/hsed').name, 'notfound');
  assert.equal(match('/nowhere').name, 'notfound');
});
