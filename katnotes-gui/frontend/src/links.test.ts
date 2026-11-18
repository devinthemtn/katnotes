import { test } from 'node:test';
import assert from 'node:assert/strict';
import { slugify, linkMarkdown, linkQuery, linkSuggestions, applyLink, resolveHref, decorateLinks, type NoteRef } from './links.ts';

const now = new Date(2026, 10, 17, 15, 0); // Tuesday
const notes: NoteRef[] = [
  { kind: 'page', id: 'groceries', title: 'Groceries' },
  { kind: 'page', id: 'grow-lights', title: 'Grow lights' },
  { kind: 'page', id: 'plan', title: 'Plan' },
  { kind: 'journal', id: '2026-11-17', title: 'Nov 17, 2026' },
];

test('slugify matches Go', () => {
  // Same cases as TestSlugify in internal/notes/store_test.go.
  const cases: Record<string, string> = {
    'Hello World': 'hello-world', '  Go: TUI & Wails!! ': 'go-tui-wails', 'Café notes': 'café-notes',
    '***': 'untitled', 'a/b\\c..d': 'a-b-c-d',
  };
  for (const [i, w] of Object.entries(cases)) assert.equal(slugify(i), w, i);
});

test('linkMarkdown and linkQuery', () => {
  assert.equal(linkMarkdown('page', 'a-b', 'A [b]'), '[A \\[b\\]](../pages/a-b.md)');
  assert.equal(linkMarkdown('journal', '2026-11-17', '2026-11-17'), '[2026-11-17](../journal/2026-11-17.md)');
  assert.deepEqual(linkQuery('call about [[gro'), { q: 'gro', start: 11 });
  assert.deepEqual(linkQuery('[['), { q: '', start: 0 });
  assert.equal(linkQuery('call [gro'), null);
  assert.equal(linkQuery('`code [[x'), null);
  assert.equal(linkQuery('[[a]] b'), null);
});

test('linkSuggestions', () => {
  const labels = (q: string) => linkSuggestions(q, notes, now).map((s) => s.label);
  assert.deepEqual(labels('gro'), ['Groceries', 'Grow lights', 'New page "gro"']);
  assert.deepEqual(labels('groceries'), ['Groceries']);
  assert.deepEqual(labels(''), ['2026-11-17', 'Groceries', 'Grow lights', 'Plan']);
  const fri = linkSuggestions('fri', notes, now)[0];
  assert.deepEqual([fri.kind, fri.id, fri.detail], ['journal', '2026-11-20', 'new journal entry']);
  const np = linkSuggestions('Big Idea', notes, now).at(-1)!;
  assert.deepEqual([np.isNew, np.id, np.title], [true, 'big-idea', 'Big Idea']);
});

test('applyLink', () => {
  const md = '[Groceries](../pages/groceries.md)';
  assert.deepEqual(applyLink('buy [[gro', 9, 4, md), { line: 'buy ' + md, col: 4 + md.length });
  assert.deepEqual(applyLink('buy [[gro]] now', 9, 4, md), { line: 'buy ' + md + ' now', col: 4 + md.length });
});

test('resolveHref', () => {
  assert.deepEqual(resolveHref('../pages/groceries.md', 'page'), { kind: 'page', id: 'groceries' });
  assert.deepEqual(resolveHref('other.md#part', 'page'), { kind: 'page', id: 'other' });
  assert.deepEqual(resolveHref('2026-11-16.md', 'journal'), { kind: 'journal', id: '2026-11-16' });
  assert.deepEqual(resolveHref('../pages/caf%C3%A9.md', 'journal'), { kind: 'page', id: 'café' });
  for (const h of ['https://x.com/a.md', 'mailto:a@b', '/abs.md', '../../x.md', '../journal/nope.md', '#top', 'a.txt'])
    assert.equal(resolveHref(h, 'page'), null, h);
});

test('decorateLinks', () => {
  const html = '<p><a href="../pages/groceries.md">G</a> <a href="../pages/nope.md">N</a> <a href="https://x.com">X</a></p>';
  const out = decorateLinks(html, 'page', (k, id) => notes.some((n) => n.kind === k && n.id === id));
  assert.match(out, /<a class="note-link" data-kind="page" data-id="groceries" href="\.\.\/pages\/groceries\.md">/);
  assert.match(out, /class="note-link missing" data-kind="page" data-id="nope"/);
  assert.match(out, /<a href="https:\/\/x\.com">X/);
});
