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
export function decorateTasks(html: string): string {
  let n = 0;
  return html
    .split(/(<pre[\s\S]*?<\/pre>)/)
    .map((part) => {
      if (part.startsWith('<pre')) return part;
      return part
        .replace(/(<p>|\n)\[( |x|X)\] /g, (_m, lead: string, mark: string) => lead + TASK_INPUT(mark !== ' ') + ' ')
        .replace(/<input type="checkbox" class="task"/g, () => `<input type="checkbox" class="task" data-task="${n++}"`);
    })
    .join('');
}

const TASK_LINE = /^(\s*(?:>\s*)*(?:(?:[-*+]|\d+[.)])\s+)?)\[( |x|X)\](?=\s)/;
const FENCE = /^\s*(```|~~~)/;

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
  lines[idx] = lines[idx].replace(TASK_LINE, (_m, pre: string, mark: string) => `${pre}[${mark === ' ' ? 'x' : ' '}]`);
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
  if (TASK_LINE.test(line)) {
    return line.replace(TASK_LINE, (_m, pre: string, mark: string) => `${pre}[${mark === ' ' ? 'x' : ' '}]`);
  }
  return addBox(line).line;
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
  const textOf = (i: number) => lines[i].replace(TASK_LINE, '').trim();
  let idx = tasks.includes(line) && (text === undefined || textOf(line) === text.trim()) ? line : -1;
  if (idx < 0 && text !== undefined) {
    const same = tasks.filter((i) => textOf(i) === text.trim());
    if (same.length === 1) idx = same[0];
  }
  if (idx < 0) return null;
  lines[idx] = lines[idx].replace(TASK_LINE, (_m, pre: string) => `${pre}[${done ? 'x' : ' '}]`);
  return lines.join('\n');
}
