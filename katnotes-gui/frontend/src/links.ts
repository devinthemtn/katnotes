// Links between notes. They are stored as plain relative Markdown links,
// "[Groceries](../pages/groceries.md)", so katdown and GitHub can follow them;
// typing "[[" in the editor opens a picker that writes one. Mirrors
// internal/notes/links.go.
import { rank } from './slash.ts';
import { normalizeDate, today } from './tasks.ts';

export type NoteRef = { kind: string; id: string; title: string };
export type LinkSuggestion = NoteRef & { label: string; detail: string; isNew?: boolean };

const DIR: Record<string, string> = { page: 'pages', journal: 'journal' };
const KIND_OF_DIR: Record<string, string> = { pages: 'page', journal: 'journal' };
const JOURNAL_ID = /^\d{4}-\d{2}-\d{2}$/;
const MAX = 8;

// Go's Slugify: lowercase letters and digits (any script) joined by single dashes.
export function slugify(title: string): string {
  let out = '';
  let dash = false;
  for (const ch of title.toLowerCase()) {
    if (/[\p{L}\p{N}]/u.test(ch)) {
      out += ch;
      dash = false;
    } else if (!dash && out) {
      out += '-';
      dash = true;
    }
  }
  out = out.replace(/^-+|-+$/g, '');
  return out || 'untitled';
}

export const linkDest = (kind: string, id: string) => `../${DIR[kind] ?? 'pages'}/${id}.md`;

export function linkMarkdown(kind: string, id: string, text: string): string {
  return `[${text.replace(/[\\[\]]/g, (c) => '\\' + c)}](${linkDest(kind, id)})`;
}

// The "[[query" the caret is in, if any (not inside inline code).
export function linkQuery(before: string): { q: string; start: number } | null {
  const m = before.match(/\[\[([^[\]\n]*)$/);
  if (!m || m.index === undefined) return null;
  if ((before.slice(0, m.index).match(/`/g) ?? []).length % 2 === 1) return null;
  return { q: m[1], start: m.index };
}

// Picker items for a query: a journal date when the query reads as one (or
// today's entry for an empty query), matching notes, then "New page".
export function linkSuggestions(q: string, notes: NoteRef[], now = new Date()): LinkSuggestion[] {
  const query = q.trim();
  const out: LinkSuggestion[] = [];
  const has = (kind: string, id: string) => notes.some((n) => n.kind === kind && n.id === id);
  const date = query ? normalizeDate(query, now) : today(now);
  if (date) {
    out.push({ kind: 'journal', id: date, title: date, label: date, detail: has('journal', date) ? 'journal' : 'new journal entry' });
  }
  const ranked = rank(
    notes.map((n) => ({ name: n.title.toLowerCase(), n })),
    query.toLowerCase(),
    (x) => x.n.id,
  );
  for (const { n } of ranked) {
    if (out.length >= MAX) break;
    if (n.kind === 'journal' && n.id === date) continue;
    out.push({ ...n, label: n.title, detail: n.kind === 'journal' ? `journal ${n.id}` : 'page' });
  }
  if (query && !JOURNAL_ID.test(query) && !notes.some((n) => n.title.toLowerCase() === query.toLowerCase())) {
    const id = slugify(query);
    const existing = notes.find((n) => n.kind === 'page' && n.id === id);
    if (!existing) {
      out.splice(Math.min(out.length, MAX - 1), 0, { kind: 'page', id, title: query, label: `New page "${query}"`, detail: `pages/${id}.md`, isNew: true });
    }
  }
  return out.slice(0, MAX);
}

// Link text for a suggestion: journal entries are linked by date.
export const linkText = (s: NoteRef) => (s.kind === 'journal' ? s.id : s.title);

// Replace "[[query" (start..col), and a "]]" right after the caret, with md.
export function applyLink(line: string, col: number, start: number, md: string): { line: string; col: number } {
  let tail = line.slice(col);
  if (tail.startsWith(']]')) tail = tail.slice(2);
  return { line: line.slice(0, start) + md + tail, col: start + md.length };
}

// The note a link destination points at, from a note of kind `from`.
export function resolveHref(href: string, from: string): { kind: string; id: string } | null {
  let d = href.trim().replace(/^<|>$/g, '');
  d = d.split(/[#?]/)[0];
  if (!d || d.includes(':') || d.startsWith('/') || d.startsWith('\\')) return null;
  try {
    d = decodeURIComponent(d);
  } catch {
    /* keep as written */
  }
  const parts: string[] = [DIR[from] ?? 'pages'];
  for (const seg of d.split('/')) {
    if (seg === '' || seg === '.') continue;
    if (seg === '..') {
      if (!parts.length) return null;
      parts.pop();
    } else parts.push(seg);
  }
  if (parts.length !== 2 || !parts[1].endsWith('.md')) return null;
  const kind = KIND_OF_DIR[parts[0]];
  const id = parts[1].slice(0, -3);
  if (!kind || !id || id.startsWith('.') || (kind === 'journal' && !JOURNAL_ID.test(id))) return null;
  return { kind, id };
}

const unescapeAttr = (s: string) =>
  s.replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');

// Mark links to notes in rendered HTML: class "note-link" (plus "missing"
// when the note doesn't exist) and data-kind / data-id for the click handler.
export function decorateLinks(html: string, from: string, exists: (kind: string, id: string) => boolean): string {
  return html.replace(/<a href="([^"]*)"/g, (m, href: string) => {
    const t = resolveHref(unescapeAttr(href), from);
    if (!t) return m;
    const cls = exists(t.kind, t.id) ? 'note-link' : 'note-link missing';
    return `<a class="${cls}" data-kind="${t.kind}" data-id="${t.id.replace(/"/g, '&quot;')}" href="${href}"`;
  });
}
