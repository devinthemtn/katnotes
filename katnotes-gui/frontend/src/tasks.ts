// Markdown task helpers. The note file stays plain GFM (`- [ ] item`,
// `- [x] item`); everything here either renders it or edits that text.

const TASK_INPUT = (checked: boolean) =>
  `<input type="checkbox" class="task"${checked ? ' checked' : ''}>`;

// marked renderer override for GFM list-item checkboxes.
export function checkboxHTML({ checked }: { checked: boolean }): string {
  return TASK_INPUT(checked) + ' ';
}

// Turn bare `[ ]` / `[x]` at the start of a paragraph line into task inputs
// (GFM only does this inside list items), then number every task input in
// document order. <pre> blocks are left untouched.
//
// With links (each task's {kt:…} value from taskLinks), every task also gets
// a katTasks link button at its end, and the kt chip is left out.
export function decorateTasks(html: string, links?: string[]): string {
  let n = 0;
  return html
    .split(/(<pre[\s\S]*?<\/pre>)/)
    .map((part) => {
      if (part.startsWith('<pre')) return part;
      let out = part
        .replace(/(<p>|\n)\[( |x|X)\] /g, (_m, lead: string, mark: string) => lead + TASK_INPUT(mark !== ' ') + ' ')
        .replace(/<input type="checkbox" class="task"/g, () => `<input type="checkbox" class="task" data-task="${n++}"`)
        .replace(TASK_META_HTML, (m, lead: string, body: string, tail: string) => {
          let pairs = parseMetaBody(unescapeHTML(body));
          if (!pairs) return m;
          if (links) pairs = pairs.filter((p) => p.key !== 'kt');
          return lead.replace(/\s+$/, '') + (pairs.length ? ' ' + metaChips(pairs) : '') + tail;
        });
      if (links) {
        out = out.replace(TASK_END_HTML, (_m, head: string, idx: string, tail: string) =>
          head.replace(/\s+$/, '') + ' ' + ktButton(Number(idx), links[Number(idx)] ?? '') + tail);
      }
      return out;
    })
    .join('');
}

// The end of a rendered task's own text (as TASK_META_HTML), with its index.
const TASK_END_HTML = /(<input type="checkbox" class="task" data-task="(\d+)"[^>]*>(?:(?!<\/li>|<input)[^\n])*?)(\s*(?:<\/p>|<\/li>|<ul|<ol|\n|$))/g;

// The preview's katTasks button for task n with {kt:value}.
function ktButton(n: number, value: string): string {
  const [cls, label, title] =
    value === '' ? ['', '⇄', 'Link to katTasks (todo.txt)']
    : value === 'new' ? [' pending', '⇄…', 'Links to katTasks when the note is saved: click to link now']
    : [' linked', '⇄ ✓', `Linked to katTasks (kt:${value}): click to write its todo.txt line again`];
  return `<button type="button" class="kt-link${cls}" data-kt-task="${n}" title="${escapeHTML(title)}">${label}</button>`;
}

// Each task's {kt:…} value, in the order of taskLines ('' when not linked).
export function taskLinks(body: string): string[] {
  const lines = body.split('\n');
  return taskLines(body).map((i) => {
    const text = lines[i].replace(TASK_LINE, '').trim();
    return splitMeta(text).pairs.find((p) => p.key === 'kt')?.value ?? '';
  });
}

// A {…} block at the end of a rendered task's text: after the task input,
// before the end of the line, paragraph or list item, or a nested list.
const TASK_META_HTML = /(<input type="checkbox" class="task"[^>]*>(?:(?!<\/li>|<input)[^\n])*?)\{([^{}<>\n]*)\}(\s*(?:<\/p>|<\/li>|<ul|<ol|\n|$))/g;

const escapeHTML = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
export const unescapeHTML = (s: string) =>
  s.replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');

// Relative state of a date value against today: 'past', 'today' or 'future'.
export function dateState(d: string, now = today()): 'past' | 'today' | 'future' | '' {
  if (!DATE_RE.test(d)) return '';
  return d < now ? 'past' : d === now ? 'today' : 'future';
}

// The preview's chips for a metadata block. The note text is not changed.
export function metaChips(pairs: MetaPair[]): string {
  const chips = pairs.map(({ key, value }) => {
    let cls = `meta-chip meta-${/^[a-z0-9_-]+$/.test(key) ? key : 'x'}`;
    let label = `${key} ${value}`;
    if (key === 'due') {
      const st = dateState(value);
      if (st === 'past') cls += ' overdue';
      if (st === 'today') {
        cls += ' due-today';
        label = 'due today';
      }
    } else if (key === 'pri') {
      cls += ` pri-${/^(high|med|low)$/.test(value) ? value : 'other'}`;
      label = `${value} priority`;
    } else if (key === 'kt') {
      label = value === 'new' ? 'katTasks (on save)' : 'katTasks';
    } else if (key === 'start' && dateState(value) === 'future') {
      cls += ' not-started';
      label = `starts ${value}`;
    }
    return `<span class="${cls}" title="${escapeHTML(key + ':' + value)}">${escapeHTML(label)}</span>`;
  });
  return `<span class="task-meta">${chips.join('')}</span>`;
}

export const TASK_LINE = /^(\s*(?:>\s*)*(?:(?:[-*+]|\d+[.)])\s+)?)\[( |x|X)\](?=\s)/;
const FENCE = /^\s*(```|~~~)/;

// ---- task metadata: a {key:value …} block at the end of a task line ----
// Mirrors internal/notes/taskmeta.go: only a block at the very end counts,
// every token must be key:value (keys [a-z][a-z0-9_-]*, values unquoted
// without spaces or "double quoted"), and a block inside an inline code span
// is ignored. Anything else stays plain text.

export type MetaPair = { key: string; value: string };

const META_BLOCK = /\{([^{}]*)\}\s*$/;
const META_PAIR = /^([a-z][a-z0-9_-]*):(?:"([^"]*)"|([^\s"{}]+))/;
export const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

function parseMetaBody(s: string): MetaPair[] | null {
  s = s.trim();
  if (!s) return null;
  const out: MetaPair[] = [];
  while (s) {
    const m = s.match(META_PAIR);
    if (!m) return null;
    out.push({ key: m[1], value: m[3] !== undefined ? m[3] : m[2] });
    const rest = s.slice(m[0].length);
    if (rest && rest[0] !== ' ' && rest[0] !== '\t') return null;
    s = rest.replace(/^[ \t]+/, '');
  }
  return out;
}

// Split task text into the text before its metadata block and the pairs.
// `at` is where the block starts, or -1 when there is none.
export function splitMeta(text: string): { plain: string; pairs: MetaPair[]; at: number } {
  const m = text.match(META_BLOCK);
  if (!m || m.index === undefined) return { plain: text, pairs: [], at: -1 };
  const before = text.slice(0, m.index);
  if ((before.match(/`/g) ?? []).length % 2 === 1) return { plain: text, pairs: [], at: -1 };
  const pairs = parseMetaBody(m[1]);
  if (!pairs) return { plain: text, pairs: [], at: -1 };
  return { plain: before.replace(/[ \t]+$/, ''), pairs, at: m.index };
}

// Task text with its metadata block removed.
export function stripMeta(text: string): string {
  return splitMeta(text).plain;
}

export function formatMeta(pairs: MetaPair[]): string {
  return (
    '{' +
    pairs
      .map(({ key, value }) => key + ':' + (value === '' || /[\s{}"]/.test(value) ? '"' + value.replace(/"/g, "'") + '"' : value))
      .join(' ') +
    '}'
  );
}

// Set (or with value '' remove) one key in task text's metadata block.
function withMeta(text: string, key: string, value: string): string {
  const { plain: p, pairs, at } = splitMeta(text);
  const plain = at < 0 ? text.replace(/[ \t]+$/, '') : p;
  const out: MetaPair[] = [];
  let found = false;
  for (const pr of pairs) {
    if (pr.key !== key) out.push(pr);
    else {
      if (!found && value !== '') out.push({ key, value });
      found = true;
    }
  }
  if (!found && value !== '') out.push({ key, value });
  if (!out.length) return plain;
  return plain ? plain + ' ' + formatMeta(out) : formatMeta(out);
}

export const TASK_TEXT = /^(\s*(?:>\s*)*(?:(?:[-*+]|\d+[.)])\s+)?\[(?: |x|X)\]\s+)(.*)$/;

// Set one metadata key on a task line. Returns null when it isn't a task.
export function setLineMeta(line: string, key: string, value: string): string | null {
  const m = line.match(TASK_TEXT);
  if (!m) return null;
  return m[1] + withMeta(m[2], key, value);
}

export function today(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

export const WEEKDAYS = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];

// Turn a typed date into YYYY-MM-DD, like Go's notes.NormalizeDate: ISO
// dates, today, tomorrow (tom), yesterday, +Nd / +Nw, and weekday names (the
// next such day, never today). Returns '' for "", "none", "-" and null when
// it can't be read.
export function normalizeDate(s: string, now = new Date()): string | null {
  const v = s.trim().toLowerCase();
  const day = (n: number) => today(new Date(now.getFullYear(), now.getMonth(), now.getDate() + n));
  switch (v) {
    case '': case 'none': case '-': return '';
    case 'today': case 'tod': return day(0);
    case 'tomorrow': case 'tom': case 'tmr': return day(1);
    case 'yesterday': return day(-1);
  }
  const rel = v.match(/^\+(\d+)([dw])$/);
  if (rel) return day(Number(rel[1]) * (rel[2] === 'w' ? 7 : 1));
  const wd = WEEKDAYS.indexOf(v.slice(0, 3));
  if (v.length >= 3 && wd >= 0) {
    const d = (wd - now.getDay() + 7) % 7 || 7;
    return day(d);
  }
  if (DATE_RE.test(v)) {
    const [y, m, d] = v.split('-').map(Number);
    const dt = new Date(y, m - 1, d);
    if (dt.getFullYear() === y && dt.getMonth() === m - 1 && dt.getDate() === d) return v;
  }
  return null;
}

// Make `line` a task with a trailing metadata block and return where the
// caret should go: right after "key:" (at the start of its value when the key
// is already there), or inside the block ready for a new key when key is
// omitted. Used by /due, /pri, /start and Ctrl+Space.
export function ensureMetaBlock(line: string, key?: string): { line: string; col: number } {
  if (/^\s*(?:>\s*)*(?:(?:[-*+]|\d+[.)])\s+)?\[( |x|X)\]$/.test(line)) line += ' ';
  if (!TASK_TEXT.test(line)) line = addBox(line).line;
  const m = line.match(TASK_TEXT)!;
  const pre = m[1];
  const { plain: p, pairs, at } = splitMeta(m[2]);
  const plain = (at < 0 ? m[2] : p).replace(/[ \t]+$/, '');
  const lead = pre + (plain ? plain + ' ' : '');
  // Rebuild the block (normalizing only its spacing), noting where key's value starts.
  let body = '';
  let valueAt = -1;
  for (const pr of pairs) {
    const piece = formatMeta([pr]).slice(1, -1);
    if (body) body += ' ';
    if (pr.key === key && valueAt < 0) valueAt = body.length + key.length + 1;
    body += piece;
  }
  if (key && valueAt < 0) {
    if (body) body += ' ';
    body += key + ':';
    valueAt = body.length;
  }
  if (!key) {
    if (body) body += ' ';
    valueAt = body.length;
  }
  return { line: lead + '{' + body + '}', col: lead.length + 1 + valueAt };
}

// Whether checking a task from the GUI stamps done:<date> (config task_done_date).
let stampDone = true;
export function setStampDone(on: boolean) {
  stampDone = on;
}

// Set a task line's box to done/undone, stamping or removing done:<date>.
function markLine(line: string, done: boolean): string {
  let out = line.replace(TASK_LINE, (_m, pre: string) => `${pre}[${done ? 'x' : ' '}]`);
  if (stampDone) out = setLineMeta(out, 'done', done ? today() : '') ?? out;
  return out;
}

// Source line index of each task marker, in the same order decorateTasks numbers them.
export function taskLines(body: string): number[] {
  const out: number[] = [];
  let fence = '';
  body.split('\n').forEach((line, i) => {
    const f = line.match(FENCE);
    if (f) {
      if (!fence) fence = f[1];
      else if (f[1] === fence) fence = '';
      return;
    }
    if (!fence && TASK_LINE.test(line)) out.push(i);
  });
  return out;
}

// Flip the nth task in body. Returns null if there is no such task.
export function toggleTask(body: string, n: number): string | null {
  const idx = taskLines(body)[n];
  if (idx === undefined) return null;
  const lines = body.split('\n');
  const mark = lines[idx].match(TASK_LINE)![2];
  lines[idx] = markLine(lines[idx], mark === ' ');
  return lines.join('\n');
}

// Insert a task box into a line that isn't a task yet. Returns the new line,
// where the insertion happened, and how many characters were added there.
function addBox(line: string): { line: string; at: number; added: number; dropped: number } {
  const list = line.match(/^(\s*(?:[-*+]|\d+[.)])\s+)/);
  const indent = line.match(/^\s*/)![0];
  const at = list ? list[1].length : indent.length;
  const insert = list ? '[ ] ' : '- [ ] ';
  let tail = line.slice(at);
  let dropped = 0;
  // A bare "-" / "*" / "+" marker with nothing typed after it.
  if (!list && /^[-*+]\s*$/.test(tail)) {
    dropped = tail.length;
    tail = '';
  }
  return { line: line.slice(0, at) + insert + tail, at, added: insert.length, dropped };
}

// Ctrl+Enter: make the line a task, or check/uncheck it if it already is one.
export function makeTask(line: string): string {
  // "- [ ]" whose trailing space was stripped (GFM needs it): restore it first.
  if (/^\s*(?:>\s*)*(?:(?:[-*+]|\d+[.)])\s+)?\[( |x|X)\]$/.test(line)) line += ' ';
  if (TASK_LINE.test(line)) {
    return markLine(line, line.match(TASK_LINE)![2] === ' ');
  }
  return addBox(line).line;
}

// A list or task line: lead (indent and > quotes), bullet or number, and an
// optional box. A plain item needs a space after its bullet ("- "), so a
// lone "-" or a "---" rule isn't a list; a task's box may lack the space.
const LIST_PARTS = /^(\s*(?:>\s*)*)(?:((?:[-*+]|\d+[.)])\s+)(?:(\[(?: |x|X)\])(?:\s+|$))?|(\[(?: |x|X)\])(?:\s+|$))/;

// Enter on a list item or task line. An empty item is removed (the line is
// cleared, so Enter leaves the list); otherwise the line is split at `col`
// and the new line gets the same indent and bullet, the next number in a
// numbered list, and an unchecked box when the line is a task. Returns the
// text that replaces the line and the cursor column in it, or null for a
// normal Enter (not a list, or the cursor is inside the bullet or box).
export function continueList(line: string, col: number): { text: string; col: number } | null {
  const m = line.match(LIST_PARTS);
  if (!m) return null;
  const head = m[0].length;
  if (line.slice(head).trim() === '') return { text: '', col: 0 };
  if (col < head) return null;
  let bullet = m[2] ?? '';
  const num = bullet.match(/^(\d+)([.)])(\s+)$/);
  if (num) bullet = String(Number(num[1]) + 1) + num[2] + num[3];
  const prefix = m[1] + bullet + (m[3] || m[4] ? '[ ] ' : '');
  const before = line.slice(0, col).replace(/\s+$/, '');
  const text = (col === head ? line.slice(0, head) : before) + '\n' + prefix + line.slice(col).replace(/^\s+/, '');
  return { text, col: (col === head ? head : before.length) + 1 + prefix.length };
}

// Rewrite one line for the /todo trigger. `col` is the cursor column, right
// after the "/todo" token. Returns the new line and cursor column, or null if
// the text before the cursor doesn't end with a standalone /todo.
export function applyTodo(line: string, col: number): { line: string; col: number } | null {
  const before = line.slice(0, col);
  if (!/(^|\s)\/todo$/.test(before)) return null;
  const tokenStart = col - '/todo'.length;
  let rest = line.slice(0, tokenStart) + line.slice(col);
  const cursor = tokenStart;
  // Drop the space the token leaves behind mid-line ("buy /todo milk").
  if (cursor > 0 && rest[cursor - 1] === ' ' && rest[cursor] === ' ') {
    rest = rest.slice(0, cursor) + rest.slice(cursor + 1);
  }
  if (TASK_LINE.test(rest)) return { line: rest, col: cursor };

  const r = addBox(rest);
  const c = cursor <= r.at ? r.at + r.added : cursor + r.added - r.dropped;
  return { line: r.line, col: Math.min(Math.max(c, r.at + r.added), r.line.length) };
}

// Set the task on body line `line` to done/undone. If that line isn't the
// expected task any more (unsaved edits moved it), fall back to the single
// task whose text matches `text`. Returns null when it can't be found.
export function setTaskLine(body: string, line: number, done: boolean, text?: string): string | null {
  const lines = body.split('\n');
  const tasks = taskLines(body);
  // Compare text without metadata, like the Go side's Task.Text.
  const textOf = (i: number) => stripMeta(lines[i].replace(TASK_LINE, '').trim());
  let idx = tasks.includes(line) && (text === undefined || textOf(line) === text.trim()) ? line : -1;
  if (idx < 0 && text !== undefined) {
    const same = tasks.filter((i) => textOf(i) === text.trim());
    if (same.length === 1) idx = same[0];
  }
  if (idx < 0) return null;
  lines[idx] = markLine(lines[idx], done);
  return lines.join('\n');
}

// Set metadata `key` on the task at body line `line` (matching `text` like
// setTaskLine). Returns null when the task can't be found.
export function setTaskMetaLine(body: string, line: number, key: string, value: string, text?: string): string | null {
  const lines = body.split('\n');
  const tasks = taskLines(body);
  const textOf = (i: number) => stripMeta(lines[i].replace(TASK_LINE, '').trim());
  let idx = tasks.includes(line) && (text === undefined || textOf(line) === text.trim()) ? line : -1;
  if (idx < 0 && text !== undefined) {
    const same = tasks.filter((i) => textOf(i) === text.trim());
    if (same.length === 1) idx = same[0];
  }
  if (idx < 0) return null;
  const next = setLineMeta(lines[idx], key, value);
  if (next === null) return null;
  lines[idx] = next;
  return lines.join('\n');
}
