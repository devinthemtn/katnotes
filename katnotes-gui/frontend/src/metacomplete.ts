// Autocomplete for a task's {key:value} metadata block in the editor. Pure
// string logic: App.svelte owns the textarea and the dropdown.
import { TASK_TEXT, WEEKDAYS, normalizeDate, today } from './tasks.ts';

// Mirrors notes.MetaKeyStat from the Go side.
export type MetaStat = { key: string; count: number; values: { value: string; count: number }[] | null };

export type MetaContext =
  | { kind: 'key'; from: number; to: number; prefix: string; used: string[] }
  | { kind: 'value'; key: string; from: number; to: number; prefix: string };

export type Suggestion = {
  label: string; // what the menu shows first
  detail: string; // greyed text after it
  insert: string; // replaces line[from, to)
  then?: 'value'; // a key: reopen the menu for its value
};

const KEY_RE = /^[a-z][a-z0-9_-]*$/;
export const DATE_KEYS = new Set(['due', 'start', 'done']);
const BUILTIN_KEYS: [string, string][] = [
  ['due', 'due date'],
  ['pri', 'priority'],
  ['start', 'start date'],
];
const LINK_KEY: [string, string] = ['kt', 'link to katTasks (todo.txt)'];

// Whether [todo_txt] is on, so kt (link to katTasks) is offered as a key.
let todoLinks = false;
export function setTodoLinks(on: boolean) {
  todoLinks = on;
}
const MAX_ITEMS = 8;

// Where the caret is inside a task's trailing {…} block, or null when the
// menu shouldn't open (not a task, inside code or quotes, block not last…).
export function metaContext(line: string, col: number): MetaContext | null {
  const m = line.match(TASK_TEXT);
  if (!m) return null;
  const textStart = m[1].length;
  const open = line.lastIndexOf('{', col - 1);
  if (open < textStart) return null;
  const before = line.slice(open + 1, col);
  if (before.includes('}')) return null; // caret is past a closed block
  if ((line.slice(0, open).match(/`/g) ?? []).length % 2 === 1) return null; // inline code
  if ((before.match(/"/g) ?? []).length % 2 === 1) return null; // inside a quoted value
  const rest = line.slice(col);
  const close = rest.indexOf('}');
  if (close >= 0) {
    if (rest.slice(close + 1).trim() !== '' || rest.slice(0, close).includes('{')) return null;
  } else if (!/^\S*\s*$/.test(rest) || rest.includes('{')) {
    return null;
  }
  const inner = line.slice(open + 1, close >= 0 ? col + close : line.length);
  const tokStart = Math.max(line.lastIndexOf(' ', col - 1), line.lastIndexOf('\t', col - 1), open) + 1;
  const typed = line.slice(tokStart, col);
  const colon = typed.indexOf(':');
  if (colon >= 0) {
    const key = typed.slice(0, colon);
    if (!KEY_RE.test(key)) return null;
    const right = rest.match(/^[^\s}]*/)![0];
    return { kind: 'value', key, from: tokStart + colon + 1, to: col + right.length, prefix: typed.slice(colon + 1) };
  }
  if (!/^[a-z0-9_-]*$/i.test(typed)) return null;
  // Only offer keys at the end of a token, not in front of existing text.
  if (rest && !/^[\s}]/.test(rest)) return null;
  const used = inner
    .split(/\s+/)
    .map((t) => t.split(':')[0])
    .filter((k, i, a) => KEY_RE.test(k) && a.indexOf(k) === i && k !== typed);
  return { kind: 'key', from: tokStart, to: col, prefix: typed.toLowerCase(), used };
}

// Quote a value the way formatMeta does.
export function quoteValue(v: string): string {
  return v === '' || /[\s{}"]/.test(v) ? '"' + v.replace(/"/g, "'") + '"' : v;
}

const SHORT_DAY = (iso: string) => {
  const [y, m, d] = iso.split('-').map(Number);
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' });
};

function dateSuggestions(prefix: string, now: Date): Suggestion[] {
  const p = prefix.trim().toLowerCase();
  const out: Suggestion[] = [];
  const seen = new Set<string>();
  const add = (label: string, iso: string | null) => {
    if (!iso || seen.has(label)) return;
    seen.add(label);
    out.push({ label, detail: iso === label ? SHORT_DAY(iso) : `${iso} · ${SHORT_DAY(iso)}`, insert: iso });
  };
  // What the typed text means, first (e.g. "+3d", "friday", "2026-12-01").
  if (p) add(p, normalizeDate(p, now));
  const shortcuts = ['today', 'tomorrow', ...WEEKDAYS.map((_, i) => WEEKDAYS[(now.getDay() + 1 + i) % 7]), '+1w', '+2w'];
  for (const s of shortcuts) if (!p || s.startsWith(p)) add(s, normalizeDate(s, now));
  // A partly typed ISO date: offer today's date when it still fits.
  const t = today(now);
  if (p && /^\d/.test(p) && t.startsWith(p)) add(t, t);
  return out;
}

// The menu items for a context.
export function suggest(ctx: MetaContext, stats: MetaStat[], now = new Date()): Suggestion[] {
  if (ctx.kind === 'key') {
    const out: Suggestion[] = [];
    const used = new Set(ctx.used);
    const counts = new Map(stats.map((s) => [s.key, s.count]));
    for (const [k, desc] of todoLinks ? [...BUILTIN_KEYS, LINK_KEY] : BUILTIN_KEYS) {
      const n = counts.get(k);
      out.push({ label: k, detail: n ? `${desc} · used ${n}×` : desc, insert: k + ':', then: 'value' });
    }
    for (const s of stats) {
      if (BUILTIN_KEYS.some(([k]) => k === s.key) || s.key === 'done' || (todoLinks && s.key === 'kt')) continue;
      out.push({ label: s.key, detail: `used ${s.count}×`, insert: s.key + ':', then: 'value' });
    }
    return out.filter((s) => !used.has(s.label) && s.label.startsWith(ctx.prefix)).slice(0, MAX_ITEMS);
  }
  const p = ctx.prefix.toLowerCase();
  if (DATE_KEYS.has(ctx.key)) return dateSuggestions(ctx.prefix, now).slice(0, MAX_ITEMS);
  if (ctx.key === 'kt') {
    return 'new'.startsWith(p) ? [{ label: 'new', detail: 'gets an ID and a todo.txt line on save', insert: 'new' }] : [];
  }
  if (ctx.key === 'pri') {
    return ([['high', 'highest'], ['med', 'medium'], ['low', 'lowest']] as const)
      .filter(([v]) => v.startsWith(p))
      .map(([v, d]) => ({ label: v, detail: d, insert: v }));
  }
  const vals = stats.find((s) => s.key === ctx.key)?.values ?? [];
  return vals
    .filter((v) => v.value.toLowerCase().startsWith(p.replace(/^"/, '')) && v.value !== ctx.prefix)
    .slice(0, MAX_ITEMS)
    .map((v) => ({ label: v.value, detail: `used ${v.count}×`, insert: quoteValue(v.value) }));
}

// Apply a suggestion to the line. A value closes an unclosed block.
export function applySuggestion(line: string, ctx: MetaContext, s: Suggestion): { line: string; col: number } {
  let tail = line.slice(ctx.to);
  if (ctx.kind === 'value' && !tail.includes('}')) tail = '}' + tail.replace(/^\s+/, '');
  return { line: line.slice(0, ctx.from) + s.insert + tail, col: ctx.from + s.insert.length };
}

// Whether typing "{" at col should insert "{}" (a task line, caret after
// the box with only whitespace after it, not inside inline code).
export function shouldPairBrace(line: string, col: number): boolean {
  const m = line.match(TASK_TEXT);
  if (!m || col < m[1].length) return false;
  if (line.slice(col).trim() !== '') return false;
  return (line.slice(0, col).match(/`/g) ?? []).length % 2 === 0;
}
