// Run with: npm test (node --test; Node 22.18+/24 strips the types).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { continueList } from './tasks.ts';

// col at the "|" in s; the result is shown the same way.
function enter(s: string): string | null {
  const col = s.indexOf('|');
  const r = continueList(s.replace('|', ''), col);
  return r && r.text.slice(0, r.col) + '|' + r.text.slice(r.col);
}

test('continueList', () => {
  assert.equal(enter('- [ ] buy milk|'), '- [ ] buy milk\n- [ ] |');
  assert.equal(enter('- [x] done|'), '- [x] done\n- [ ] |');
  assert.equal(enter('  * [ ] nested|'), '  * [ ] nested\n  * [ ] |');
  assert.equal(enter('3. [ ] third|'), '3. [ ] third\n4. [ ] |');
  assert.equal(enter('> - [ ] quoted|'), '> - [ ] quoted\n> - [ ] |');
  assert.equal(enter('[ ] bare|'), '[ ] bare\n[ ] |');
  assert.equal(enter('- [ ] buy |milk {due:2026-11-20}'), '- [ ] buy\n- [ ] |milk {due:2026-11-20}');
  assert.equal(enter('- [ ] |milk'), '- [ ] \n- [ ] |milk');
  // An empty task is removed.
  assert.equal(enter('- [ ] |'), '|');
  assert.equal(enter('  - [x]|'), '|');
  assert.equal(enter('- [ ]   |  '), '|');
  // Plain list items.
  assert.equal(enter('- plain item|'), '- plain item\n- |');
  assert.equal(enter('  + nested|'), '  + nested\n  + |');
  assert.equal(enter('9) ninth|'), '9) ninth\n10) |');
  assert.equal(enter('> * quoted|'), '> * quoted\n> * |');
  assert.equal(enter('- one |two'), '- one\n- |two');
  assert.equal(enter('- |'), '|');
  assert.equal(enter('  1. |'), '|');
  // Normal Enter.
  assert.equal(enter('-|'), null);
  assert.equal(enter('---|'), null);
  assert.equal(enter('**bold**|'), null);
  assert.equal(enter('> quote|'), null);
  assert.equal(enter('-| item'), null);
  assert.equal(enter('text|'), null);
  assert.equal(enter('- [| ] milk'), null);
});

test('taskLinks and the preview katTasks buttons', async () => {
  const { decorateTasks, taskLinks } = await import('./tasks.ts');
  const body = '- [ ] plain\n- [x] new one {kt:new}\n- [ ] linked {due:2026-11-27 kt:7f3a2c}\n';
  assert.deepEqual(taskLinks(body), ['', 'new', '7f3a2c']);
  const html =
    '<ul>\n<li><input type="checkbox" class="task"> plain</li>\n' +
    '<li><input type="checkbox" class="task" checked> new one {kt:new}</li>\n' +
    '<li><input type="checkbox" class="task"> linked {due:2026-11-27 kt:7f3a2c}</li>\n</ul>\n';
  const out = decorateTasks(html, taskLinks(body));
  assert.match(out, /plain <button type="button" class="kt-link" data-kt-task="0"[^>]*>⇄<\/button><\/li>/);
  assert.match(out, /new one <button type="button" class="kt-link pending" data-kt-task="1"/);
  assert.match(out, /class="kt-link linked" data-kt-task="2" title="Linked to katTasks \(kt:7f3a2c\)/);
  assert.doesNotMatch(out, /meta-kt/); // the button replaces the kt chip
  assert.match(out, /meta-due/);
  // Sync off: no buttons, the chip stays.
  const off = decorateTasks(html);
  assert.doesNotMatch(off, /kt-link/);
  assert.match(off, /meta-kt/);
});
