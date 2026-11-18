// Slash commands typed in the editor ("/today", "/todo", …). Everything here
// produces plain text, so notes stay ordinary Markdown.
import { applyTodo, ensureMetaBlock, today } from './tasks.ts';

export type SlashCommand = {
  name: string;
  desc: () => string;
  // Text to put in place of the "/name" token; absent for line-level commands.
  text?: () => string;
  // User command from ~/.config/katnotes/commands/<name>.lua (runs in Go).
  user?: boolean;
  // Task metadata key: make the line a task and move into its {key:…} block.
  meta?: string;
};

// What a user Lua command returned (mirrors luacmd.Result).
export type UserResult = { kind: string; text: string; line: string; cursor: number };

const pad = (n: number) => String(n).padStart(2, '0');

export function isoDate(offsetDays = 0, now = new Date()): string {
  return today(new Date(now.getFullYear(), now.getMonth(), now.getDate() + offsetDays));
}

export function timeNow(now = new Date()): string {
  return `${pad(now.getHours())}:${pad(now.getMinutes())}`;
}

export const SLASH: SlashCommand[] = [
  { name: 'todo', desc: () => 'Make this line a task  - [ ]' },
  { name: 'due', desc: () => 'Set the task due date  {due:…}', meta: 'due' },
  { name: 'pri', desc: () => 'Set the task priority  {pri:…}', meta: 'pri' },
  { name: 'start', desc: () => 'Set the task start date  {start:…}', meta: 'start' },
  { name: 'today', desc: () => isoDate(0), text: () => isoDate(0) },
  { name: 'tomorrow', desc: () => isoDate(1), text: () => isoDate(1) },
  { name: 'yesterday', desc: () => isoDate(-1), text: () => isoDate(-1) },
  { name: 'time', desc: () => timeNow(), text: () => timeNow() },
];

// The partial command name being typed, if the text before the cursor ends
// with a "/word" that starts the line or follows whitespace.
export function slashQuery(before: string): string | null {
  const m = before.match(/(?:^|\s)\/([a-z0-9_-]*)$/i);
  return m ? m[1].toLowerCase() : null;
}

export function isSubseq(needle: string, hay: string): boolean {
  let i = 0;
  for (const ch of hay) if (ch === needle[i]) i++;
  return i === needle.length;
}

// Prefix matches first, then subsequence matches.
export function rank<T extends { name: string }>(items: T[], q: string, extra?: (t: T) => string): T[] {
  q = q.trim().toLowerCase();
  if (!q) return items;
  const prefix = items.filter((c) => c.name.startsWith(q));
  const fuzzy = items.filter(
    (c) => !prefix.includes(c) && (isSubseq(q, c.name) || (extra ? extra(c).toLowerCase().includes(q) : false))
  );
  return [...prefix, ...fuzzy];
}

export const matchSlash = (q: string, list: SlashCommand[] = SLASH) => rank(list, q);

// Built-ins first; a user command can't shadow a built-in name.
export function mergeSlash(user: SlashCommand[]): { list: SlashCommand[]; shadowed: string[] } {
  const builtin = new Set(SLASH.map((c) => c.name));
  const shadowed = user.filter((c) => builtin.has(c.name)).map((c) => c.name);
  return { list: [...SLASH, ...user.filter((c) => !builtin.has(c.name))], shadowed };
}

// Split the line around the "/query" token ending at col.
export function slashToken(line: string, col: number): { start: number; before: string; after: string } | null {
  const q = slashQuery(line.slice(0, col));
  if (q === null) return null;
  const start = col - q.length - 1;
  return { start, before: line.slice(0, start), after: line.slice(col) };
}

// Apply a user command's result to the line holding the token.
export function applyUserResult(line: string, col: number, r: UserResult): { line: string; col: number } | null {
  const tok = slashToken(line, col);
  if (!tok) return null;
  if (r.kind === 'line') {
    const out = r.line ?? '';
    const c = r.cursor < 0 || r.cursor === undefined ? out.length : Math.min(Math.max(0, r.cursor), out.length);
    return { line: out, col: c };
  }
  const t = r.text ?? '';
  return { line: tok.before + t + tok.after, col: tok.start + t.length };
}

// Replace the "/query" token that ends at `col` with the command's result.
export function expandSlash(line: string, col: number, cmd: SlashCommand): { line: string; col: number } | null {
  const q = slashQuery(line.slice(0, col));
  if (q === null) return null;
  const start = col - q.length - 1;
  if (cmd.meta) {
    // Drop the token (and the space it leaves), then open the metadata block.
    let rest = line.slice(0, start) + line.slice(col);
    if (start > 0 && rest[start - 1] === ' ' && (rest[start] === ' ' || start === rest.length)) {
      rest = rest.slice(0, start - 1) + rest.slice(start);
    }
    return ensureMetaBlock(rest, cmd.meta);
  }
  if (!cmd.text) {
    // Line-level command (todo): normalise the token, then reuse applyTodo.
    const withTok = line.slice(0, start) + '/todo' + line.slice(col);
    return applyTodo(withTok, start + 5);
  }
  const t = cmd.text();
  return { line: line.slice(0, start) + t + line.slice(col), col: start + t.length };
}

// Pixel position of the caret inside a textarea, relative to its padding box
// origin and before scrolling. Uses a hidden mirror element.
const MIRROR_PROPS = [
  'boxSizing', 'width', 'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
  'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
  'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'lineHeight', 'letterSpacing',
  'tabSize', 'textIndent', 'textTransform', 'wordSpacing',
] as const;

export function caretXY(ta: HTMLTextAreaElement, pos: number): { x: number; y: number; lineHeight: number } {
  const cs = getComputedStyle(ta);
  const div = document.createElement('div');
  const st = div.style as any;
  for (const p of MIRROR_PROPS) st[p] = (cs as any)[p];
  st.position = 'absolute';
  st.visibility = 'hidden';
  st.top = '0';
  st.left = '-9999px';
  st.whiteSpace = 'pre-wrap';
  st.overflowWrap = 'break-word';
  st.borderStyle = 'solid';
  st.overflow = 'hidden';
  div.textContent = ta.value.slice(0, pos);
  const mark = document.createElement('span');
  mark.textContent = '​';
  div.appendChild(mark);
  document.body.appendChild(div);
  const lh = parseFloat(cs.lineHeight) || parseFloat(cs.fontSize) * 1.4;
  const r = { x: mark.offsetLeft, y: mark.offsetTop, lineHeight: lh };
  div.remove();
  return r;
}
