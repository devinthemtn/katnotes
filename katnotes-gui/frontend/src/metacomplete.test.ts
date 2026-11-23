// Run with: npm test (node --test; Node 22.18+/24 strips the types).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { metaContext, suggest, setTodoLinks, applySuggestion, shouldPairBrace, type MetaStat } from './metacomplete.ts';
import { normalizeDate, ensureMetaBlock } from './tasks.ts';

const now = new Date(2026, 10, 17, 15, 0); // Tuesday 2026-11-17, like the Go tests
const stats: MetaStat[] = [
  { key: 'project', count: 4, values: [{ value: 'house', count: 3 }, { value: 'big garden', count: 1 }] },
  { key: 'due', count: 2, values: [] },
];
// col at the "|" in s.
const at = (s: string) => ({ line: s.replace('|', ''), col: s.indexOf('|') });

test('metaContext', () => {
  const ctx = (s: string) => { const { line, col } = at(s); return metaContext(line, col); };
  assert.deepEqual(ctx('- [ ] call {|}'), { kind: 'key', from: 12, to: 12, prefix: '', used: [] });
  assert.deepEqual(ctx('- [ ] call {pri:high d|}'), { kind: 'key', from: 21, to: 22, prefix: 'd', used: ['pri'] });
  assert.deepEqual(ctx('- [ ] call {due:fr|}'), { kind: 'value', key: 'due', from: 16, to: 18, prefix: 'fr' });
  assert.deepEqual(ctx('- [ ] call {due:|2026-01-01 pri:high}'), { kind: 'value', key: 'due', from: 16, to: 26, prefix: '' });
  assert.deepEqual(ctx('- [ ] call {due:x|'), { kind: 'value', key: 'due', from: 16, to: 17, prefix: 'x' });
  for (const s of [
    'plain {|}', // not a task
    '- [ ] call {due:x}|', // after the block
    '- [ ] `code {|}`', // inline code
    '- [ ] mid {|} more text', // not the last block
    '- [ ] q {note:"two |}', // inside quotes
    '- [ ] call {|due:x}', // in front of an existing token
    '- [ ] call {Bad Key:|}',
  ]) assert.equal(ctx(s), null, s);
});

test('suggest keys', () => {
  const keys = (s: string) => suggest(metaContext(at(s).line, at(s).col)!, stats, now).map((x) => x.label);
  assert.deepEqual(keys('- [ ] a {|}'), ['due', 'pri', 'start', 'project']);
  assert.deepEqual(keys('- [ ] a {due:x |}'), ['pri', 'start', 'project']);
  assert.deepEqual(keys('- [ ] a {p|}'), ['pri', 'project']);
  assert.match(suggest(metaContext('- [ ] a {}', 9)!, stats, now)[0].detail, /used 2×/);
});

test('suggest values', () => {
  const vals = (s: string) => suggest(metaContext(at(s).line, at(s).col)!, stats, now);
  const fri = vals('- [ ] a {due:fr|}');
  assert.deepEqual(fri.map((s) => [s.label, s.insert]), [['fri', '2026-11-20']]);
  assert.deepEqual(vals('- [ ] a {due:friday|}')[0].insert, '2026-11-20');
  assert.equal(vals('- [ ] a {due:+3d|}')[0].insert, '2026-11-20');
  assert.deepEqual(vals('- [ ] a {due:|}').slice(0, 3).map((s) => s.label), ['today', 'tomorrow', 'wed']);
  assert.deepEqual(vals('- [ ] a {pri:|}').map((s) => s.label), ['high', 'med', 'low']);
  assert.deepEqual(vals('- [ ] a {project:b|}').map((s) => s.insert), ['"big garden"']);
});

test('applySuggestion round trip', () => {
  let line = '- [ ] call {}';
  let ctx = metaContext(line, 12)!;
  let r = applySuggestion(line, ctx, suggest(ctx, stats, now)[0]); // due:
  assert.deepEqual(r, { line: '- [ ] call {due:}', col: 16 });
  ctx = metaContext(r.line, r.col)!;
  assert.equal(ctx.kind, 'value');
  r = applySuggestion(r.line, ctx, suggest(ctx, stats, now)[1]); // tomorrow
  assert.deepEqual(r, { line: '- [ ] call {due:2026-11-18}', col: 26 });
  // An unclosed block is closed by a value.
  line = '- [ ] call {pri:h';
  ctx = metaContext(line, line.length)!;
  assert.deepEqual(applySuggestion(line, ctx, suggest(ctx, stats, now)[0]), { line: '- [ ] call {pri:high}', col: 20 });
  // Replacing an existing value.
  line = '- [ ] call {due:2026-01-01 pri:high}';
  ctx = metaContext(line, 16)!;
  assert.equal(applySuggestion(line, ctx, { label: 'x', detail: '', insert: '2026-02-02' }).line, '- [ ] call {due:2026-02-02 pri:high}');
});

test('shouldPairBrace', () => {
  assert.ok(shouldPairBrace('- [ ] call ', 11));
  assert.ok(!shouldPairBrace('plain ', 6));
  assert.ok(!shouldPairBrace('- [ ] call more', 8));
  assert.ok(!shouldPairBrace('- [ ] `code ', 12));
});

test('ensureMetaBlock', () => {
  assert.deepEqual(ensureMetaBlock('call plumber', 'due'), { line: '- [ ] call plumber {due:}', col: 24 });
  assert.deepEqual(ensureMetaBlock('- [ ] call {pri:high}', 'due'), { line: '- [ ] call {pri:high due:}', col: 25 });
  assert.deepEqual(ensureMetaBlock('- [ ] call {due:2026-01-01  pri:high}', 'due'), { line: '- [ ] call {due:2026-01-01 pri:high}', col: 16 });
  assert.deepEqual(ensureMetaBlock('- [ ] call'), { line: '- [ ] call {}', col: 12 });
  assert.deepEqual(ensureMetaBlock('- [ ] call {pri:high}'), { line: '- [ ] call {pri:high }', col: 21 });
  assert.deepEqual(ensureMetaBlock('- [ ]', 'pri'), { line: '- [ ] {pri:}', col: 11 });
});

test('normalizeDate matches Go', () => {
  const cases: Record<string, string> = {
    '2026-12-01': '2026-12-01', today: '2026-11-17', tomorrow: '2026-11-18', tom: '2026-11-18',
    '+3d': '2026-11-20', '+2w': '2026-12-01', fri: '2026-11-20', Friday: '2026-11-20', tue: '2026-11-24', '': '', none: '',
  };
  for (const [i, w] of Object.entries(cases)) assert.equal(normalizeDate(i, now), w, i);
  for (const bad of ['2026-13-01', 'soon', '11/20']) assert.equal(normalizeDate(bad, now), null, bad);
});

test('kt is offered only with todo.txt links on', () => {
  const keys = (s: string) => suggest(metaContext(at(s).line, at(s).col)!, stats, now).map((x) => x.label);
  assert.ok(!keys('- [ ] a {|}').includes('kt'));
  setTodoLinks(true);
  try {
    assert.ok(keys('- [ ] a {k|}').includes('kt'));
    assert.deepEqual(suggest(metaContext(at('- [ ] a {kt:|}').line, at('- [ ] a {kt:|}').col)!, stats, now).map((x) => x.insert), ['new']);
  } finally {
    setTodoLinks(false);
  }
});
