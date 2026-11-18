<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import { checkboxHTML, decorateTasks, toggleTask, makeTask, setTaskLine, setTaskMetaLine, metaChips, setStampDone, dateState, today, ensureMetaBlock } from './tasks';
  import { metaContext, suggest, applySuggestion, shouldPairBrace, type MetaContext, type MetaStat, type Suggestion } from './metacomplete';
  import { SLASH, matchSlash, mergeSlash, slashQuery, slashToken, expandSlash, applyUserResult, caretXY, rank, type SlashCommand } from './slash';
  import { linkQuery, linkSuggestions, linkMarkdown, linkText, applyLink, decorateLinks, type NoteRef, type LinkSuggestion } from './links';
  import EditorMenu from './EditorMenu.svelte';
  import { Browser, Clipboard } from '@wailsio/runtime';
  import { NotesService as api } from '../bindings/github.com/devinthemtn/katnotes/katnotes-gui';
  import type { Note, Match, TagCount, NoteTasks, Task, Backlink } from '../bindings/github.com/devinthemtn/katnotes/internal/notes';
  import type { Config } from '../bindings/github.com/devinthemtn/katnotes/internal/config';
  import type { State as ViewStateT, Search as SavedSearch } from '../bindings/github.com/devinthemtn/katnotes/internal/viewstate';
  import type { Completion as CmdCompletion, Result as CmdResult, Choice as CmdChoice, Spec as CmdSpec } from '../bindings/github.com/devinthemtn/katnotes/internal/commands';

  marked.use({ renderer: { checkbox: checkboxHTML } });

  type Tab = 'page' | 'journal' | 'tags' | 'tasks';
  type ViewMode = 'edit' | 'split' | 'preview';

  let tab: Tab = $state('page');
  let items: Note[] = $state([]);
  let tags: TagCount[] = $state([]);
  // Notes that contain tasks (Tasks tab and the all-tasks view).
  let taskNotes: NoteTasks[] = $state([]);
  let mainView: 'note' | 'tasks' = $state('note');
  let taskFilter = $state(loadPref('taskFilter', 'open') as 'open' | 'done' | 'all');
  let taskQuery = $state('');
  // All-tasks view: group by note or one list sorted by due date, and a date filter.
  let taskSort = $state(loadPref('taskSort', 'note') as 'note' | 'due');
  let taskWhen = $state(loadPref('taskWhen', 'any') as 'any' | 'overdue' | 'week');
  let matches: Record<string, Match> = $state({});
  let tagFilter = $state('');
  let query = $state('');
  // Search box mode (shared semantics with the TUI's "/" bar, matched in Go).
  type SearchMode = 'name' | 'tags' | 'text';
  const SEARCH_MODES: SearchMode[] = ['name', 'tags', 'text'];
  const SEARCH_HINTS: Record<SearchMode, string> = {
    name: 'fuzzy title or filename',
    tags: 'tags, all must match (-tag excludes)',
    text: 'words or "a phrase" in the text',
  };
  let searchMode = $state(loadPref('searchMode', 'name') as SearchMode);
  // Sort per tab and saved searches, shared with the TUI (view.json).
  let viewState: ViewStateT = $state({ sort: {}, searches: [] });
  const SORTS: Record<string, [string, string][]> = {
    page: [['updated', 'Modified'], ['created', 'Created'], ['title', 'Title A–Z']],
    journal: [['id', 'Date'], ['updated', 'Modified'], ['title', 'Title A–Z']],
  };

  let current: Note | null = $state(null);
  let draftBody = $state('');
  let draftTags = $state('');
  let draftTitle = $state('');
  let dirty = $state(false);
  let saveState: 'saved' | 'saving' | 'dirty' | 'error' = $state('saved');
  let conflict = $state(false);
  let viewMode: ViewMode = $state(loadPref('viewMode', 'split') as ViewMode);

  let status = $state('');
  let statusErr = $state(false);
  let notesDir = $state('');
  let configPath = $state('');
  let version = $state('');

  // modals
  let newOpen = $state(false);
  let newTitle = $state('');
  let newTemplate = $state(''); // page template name, '' for a blank page
  let templates: string[] = $state([]);
  let deleteOpen = $state(false);
  let settingsOpen = $state(false);
  let helpOpen = $state(false);
  let zoomed = $state(false);

  // command palette
  let cmdOpen = $state(false);
  let cmdInput = $state('');
  let cmdSel = $state(0);
  let cmdMoved = $state(false); // selection moved with ↑↓ since the last edit
  let cmdComp = $state<CmdCompletion | null>(null);
  let cmdErr = $state('');
  let cmdHist: string[] = $state([]); // newest first
  let cmdHistIdx = $state(-1); // browsing history with ↑, or -1
  let cmdPick = $state<{ title: string; items: CmdChoice[] } | null>(null);
  let cmdCatalog: CmdSpec[] = $state([]);
  let cmdEl: HTMLInputElement | undefined = $state();

  // ":frontmatter" editor
  let fmEditOpen = $state(false);
  let fmEditText = $state('');
  let fmEditInferred = $state(false);
  let fmEditErr = $state('');
  let fmEditEl: HTMLTextAreaElement | undefined = $state();

  // editor slash menu
  // frontmatter panel above the editor
  // Starts from config.toml's frontmatter_collapsed; toggling lasts for the session.
  let fmOpen = $state(true);
  function fmValue(v: unknown): string {
    if (v === null || v === undefined) return '';
    if (Array.isArray(v)) return v.map((x) => fmValue(x)).join(', ');
    if (typeof v === 'object') return JSON.stringify(v);
    return String(v);
  }
  function fmDate(s: string): string {
    const d = new Date(s);
    return isNaN(d.getTime()) ? s : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
  }
  let fmRows = $derived.by((): [string, string][] => {
    if (!current) return [];
    const rows: [string, string][] = [
      ['title', draftTitle],
      ['tags', parseTags(draftTags).join(', ')],
      ['created', fmDate(current.created)],
      ['updated', fmDate(current.updated)],
    ];
    const extra = current.extra ?? {};
    for (const k of Object.keys(extra).sort()) rows.push([k, fmValue(extra[k])]);
    if (current.problem) rows.unshift(['⚠ problem', current.problem + ' (the whole file is shown as the body; saving adds fresh frontmatter)']);
    return rows;
  });

  let slashOpen = $state(false);
  let slashQ = $state('');
  let slashSel = $state(0);
  let slashPos = $state({ left: 0, top: 0, above: false });
  // Built-in plus user Lua commands (~/.config/katnotes/commands/*.lua).
  let allSlash: SlashCommand[] = $state(SLASH);
  let slashItems = $derived(slashOpen ? matchSlash(slashQ, allSlash).slice(0, 6) : []);

  // editor {key:value} metadata menu
  let metaOpen = $state(false);
  let metaSel = $state(0);
  let metaCtx: MetaContext | null = $state(null);
  let metaPos = $state({ left: 0, top: 0, above: false });
  let metaStats: MetaStat[] = $state([]);
  let metaItems: Suggestion[] = $derived(metaOpen && metaCtx ? suggest(metaCtx, metaStats) : []);

  // editor [[ link picker
  let linkOpen = $state(false);
  let linkSel = $state(0);
  let linkQ = $state({ q: '', start: 0 });
  let linkPos = $state({ left: 0, top: 0, above: false });
  // Every note (title and id), for the picker and for marking missing links.
  let linkTargets: NoteRef[] = $state([]);
  let linkKeys = $derived(new Set(linkTargets.map((n) => `${n.kind}/${n.id}`)));
  let linkItems: LinkSuggestion[] = $derived(linkOpen ? linkSuggestions(linkQ.q, linkTargets) : []);
  // Notes linking to the open note.
  let backlinks: Backlink[] = $state([]);

  async function loadLinkTargets() {
    try {
      linkTargets = ((await api.List('', '')) ?? []).map((n) => ({ kind: n.kind, id: n.id, title: n.title }));
    } catch {
      /* picker just has fewer notes */
    }
  }

  async function loadBacklinks() {
    const n = current;
    if (!n) {
      backlinks = [];
      return;
    }
    try {
      const bl = (await api.Backlinks(n.kind, n.id)) ?? [];
      if (current && keyOf(current) === keyOf(n)) backlinks = bl;
    } catch {
      backlinks = [];
    }
  }

  // A backlink line with its Markdown links shown as their text.
  const plainLinks = (line: string) => line.replace(/!?\[((?:\\.|[^\]\\])*)\]\([^)]*\)/g, '$1');

  async function loadMetaStats() {
    try {
      metaStats = (await api.TaskMetaStats()) ?? [];
    } catch {
      /* suggestions just lack the user's own keys */
    }
  }

  async function loadUserSlash(announce = false) {
    try {
      const res = await api.SlashCommands();
      const user: SlashCommand[] = (res.commands ?? []).map((c) => ({
        name: c.name,
        desc: () => c.description || 'user command',
        user: true,
      }));
      const { list, shadowed } = mergeSlash(user);
      allSlash = list;
      const problems = [...(res.errors ?? []), ...shadowed.map((n) => `${n}.lua: "${n}" is a built-in command`)];
      if (problems.length) flash(`commands: ${problems.join('; ')}`, true);
      else if (announce && user.length) flash(`${user.length} user command${user.length === 1 ? '' : 's'} loaded`);
    } catch (e) {
      flash(errMsg(e), true);
    }
  }
  let uiScale = $state(Number(loadPref('uiScale', '1')) || 1);
  $effect(() => {
    (document.documentElement.style as any).zoom = String(uiScale);
  });

  const isMac = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform);
  const modLabel = isMac ? '⌘' : 'Ctrl';
  const helpSections: { title: string; keys: [string, string][] }[] = [
    { title: 'Notes', keys: [
      [`${modLabel}+N`, 'New page (or journal entry on the Journal tab)'],
      [`${modLabel}+J`, "Open today's journal entry"],
      [`${modLabel}+S`, 'Save now (notes also autosave as you type)'],
      ['Enter', 'In the title field: rename the note'],
      ['/', `In the editor: slash commands (${SLASH.map((c) => c.name).join(', ')})`],
      ['/your-command', 'Lua commands from ~/.config/katnotes/commands/*.lua (:reload after editing)'],
      [`${modLabel}+Enter`, 'In the editor: make the line a task, or check/uncheck it'],
      ['[[', 'In the editor: link to a page or journal day; saved as a Markdown link'],
      ['Click a link', 'In the preview: open the note (a missing one is created first)'],
      ['{', 'On a task line: start a {key:value} block; suggests due, pri, start and your keys, then values'],
      ['Ctrl+Space', 'On a task line: open the {…} suggestions (adds the block if needed)'],
      ['Click a box', 'In the preview: check or uncheck a task'],
      ['Tab', 'In the editor: indent two spaces'],
    ]},
    { title: 'Finding', keys: [
      [`${modLabel}+K / ${modLabel}+F`, 'Focus search; Tab in the box switches Name (fuzzy title/filename) · Tags · Text'],
      [`${modLabel}+P`, 'Quick open: jump to any note by name (recent first)'],
      ['Sort', 'The list’s sort menu: modified, created or title (remembered per tab, shared with the TUI)'],
      ['Tags tab', 'Click a tag to filter Pages and Journal, or a 🔎 saved search (:search save <name>)'],
      ['clear', 'Remove the search and tag filter'],
    ]},
    { title: 'View', keys: [
      [`${modLabel}+E`, 'Cycle Edit → Split → Preview'],
      [`${modLabel}+= / ${modLabel}+-`, 'Make text larger / smaller'],
      [`${modLabel}+0`, 'Reset text size'],
      [`${modLabel}+Shift+T`, 'All tasks from every note (Esc to go back)'],
      [`${modLabel}+Shift+O`, 'Open the note in the default app (Settings: open with)'],
      [`${modLabel}+Shift+P / ${modLabel}+Shift+:`, 'Command palette'],
      [`${modLabel}+Shift+Z`, 'Zoom: hide the sidebar and top bar (again to exit)'],
    ]},
    { title: 'General', keys: [
      ['? / F1', 'Toggle this help (? works outside text fields)'],
      ['Esc', 'Close dialogs'],
    ]},
    { title: 'Outside changes', keys: [
      ['Window focus', 'Reloads notes edited by the CLI or TUI'],
      ['Conflict bar', 'Reload theirs, or keep your version'],
    ]},
  ];
  let cfg: Config | null = $state(null);

  let searchEl: HTMLInputElement | undefined = $state();
  let newEl: HTMLInputElement | undefined = $state();
  let bodyEl: HTMLTextAreaElement | undefined = $state();

  let saveTimer: ReturnType<typeof setTimeout> | undefined;
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  function loadPref(k: string, d: string): string {
    try { return localStorage.getItem('katnotes.' + k) ?? d; } catch { return d; }
  }
  function savePref(k: string, v: string) {
    try { localStorage.setItem('katnotes.' + k, v); } catch { /* ignore */ }
  }

  function errMsg(e: unknown): string {
    if (e && typeof e === 'object' && 'message' in e) return String((e as any).message);
    return String(e);
  }
  function flash(msg: string, isErr = false) {
    status = msg;
    statusErr = isErr;
    if (!isErr) setTimeout(() => { if (status === msg) status = ''; }, 3000);
  }

  const keyOf = (n: { kind: string; id: string }) => `${n.kind}/${n.id}`;

  let preview = $derived(
    DOMPurify.sanitize(
      decorateLinks(
        decorateTasks(marked.parse(draftBody, { async: false, gfm: true, breaks: false }) as string),
        (current as Note | null)?.kind ?? 'page',
        (kind, id) => linkKeys.has(`${kind}/${id}`),
      ),
    )
  );

  function parseTags(s: string): string[] {
    return s.split(/[,\s]+/).map((t) => t.replace(/^#/, '').trim().toLowerCase()).filter(Boolean);
  }

  async function refreshList() {
    try {
      if (tab === 'tasks' || mainView === 'tasks') {
        taskNotes = (await api.Tasks()) ?? [];
        if (tab === 'tasks') return;
      }
      if (tab === 'tags') {
        tags = (await api.Tags()) ?? [];
        return;
      }
      const res = (await api.Find(searchMode, query, tab, tagFilter, sortFor(tab))) ?? [];
      const m: Record<string, Match> = {};
      const list: Note[] = [];
      for (const r of res) {
        if (query.trim()) m[keyOf(r.note)] = r;
        list.push(r.note);
      }
      matches = m;
      items = list;
      if (query.trim()) {
        const all = (await api.Find('name', '', tab, tagFilter, '')) ?? [];
        totalCount = all.length;
      } else {
        totalCount = list.length;
      }
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  function setCurrent(n: Note | null) {
    slashOpen = false;
    metaOpen = false;
    linkOpen = false;
    const changed = (n ? keyOf(n) : '') !== (current ? keyOf(current) : '');
    current = n;
    if (changed) backlinks = [];
    loadBacklinks();
    if (n) savePref('lastNote', keyOf(n));
    if (n && changed) api.TouchRecent(n.kind, n.id).catch(() => {});
    conflict = false;
    dirty = false;
    saveState = 'saved';
    draftBody = n?.body ?? '';
    draftTags = (n?.tags ?? []).join(', ');
    draftTitle = n?.title ?? '';
  }

  async function open(kind: string, id: string) {
    mainView = 'note';
    if (current && keyOf(current) === `${kind}/${id}`) return;
    await flushSave();
    try {
      setCurrent(await api.Get(kind, id));
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  function onEdit() {
    dirty = true;
    saveState = 'dirty';
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => save(), 800);
  }

  async function save(force = false) {
    clearTimeout(saveTimer);
    if (!current || (!dirty && !force)) return;
    // The file changed underneath us: only "Keep mine" may overwrite it.
    if (conflict && !force) return;
    const target = current;
    saveState = 'saving';
    try {
      const n = await api.Save(target.kind, target.id, parseTags(draftTags), draftBody, target.updated, force);
      // Our own write isn't an external edit.
      if (extWatch?.key === keyOf(target)) extWatch.stamp = await api.FileStamp(target.kind, target.id);
      if (n && current && keyOf(current) === keyOf(target)) {
        current = { ...n, body: draftBody };
        dirty = false;
        conflict = false;
        saveState = 'saved';
      }
      await refreshList();
      loadMetaStats();
      loadBacklinks();
    } catch (e) {
      const msg = errMsg(e);
      saveState = 'error';
      if (msg.includes('changed outside the app')) {
        conflict = true;
      } else {
        flash(msg, true);
      }
    }
  }

  async function flushSave() {
    if (dirty && !conflict) await save();
  }

  async function reloadCurrent() {
    if (!current) return;
    try {
      setCurrent(await api.Get(current.kind, current.id));
    } catch (e) {
      setCurrent(null);
      flash(errMsg(e), true);
    }
  }

  async function commitTitle() {
    if (!current) return;
    const t = draftTitle.trim();
    if (!t || t === current.title) {
      draftTitle = current.title;
      return;
    }
    await flushSave();
    try {
      const r = await api.Rename(current.kind, current.id, t);
      if (r?.note) {
        setCurrent(r.note);
        const n = r.updated?.length ?? 0;
        flash(`renamed to ${keyOf(r.note)}${n ? ` · updated links in ${n} note${n === 1 ? '' : 's'}` : ''}`);
      }
      await refreshList();
      loadLinkTargets();
    } catch (e) {
      draftTitle = current.title;
      flash(errMsg(e), true);
    }
  }

  async function switchTab(t: Tab) {
    tab = t;
    await refreshList();
  }

  async function openNew() {
    newTitle = '';
    newTemplate = '';
    try { templates = (await api.Templates()) ?? []; } catch { templates = []; }
    newOpen = true;
    await tick();
    newEl?.focus();
  }

  async function createNote() {
    const kind = tab === 'journal' ? 'journal' : 'page';
    if (kind === 'page' && !newTitle.trim()) return;
    await flushSave();
    try {
      const tags = tagFilter ? [tagFilter] : [];
      const n = kind === 'page' && newTemplate
        ? await api.CreateFromTemplate(newTemplate, newTitle.trim(), tags)
        : await api.Create(kind, newTitle.trim(), tags);
      newOpen = false;
      if (tab === 'tags' || tab === 'tasks') tab = 'page';
      mainView = 'note';
      await refreshList();
      loadLinkTargets();
      if (n) {
        setCurrent(n);
        flash(`created ${keyOf(n)}`);
        await tick();
        bodyEl?.focus();
      }
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  async function openToday() {
    await flushSave();
    try {
      const n = await api.Today();
      tab = 'journal';
      mainView = 'note';
      await refreshList();
      if (n) {
        setCurrent(n);
        await tick();
        if (bodyEl) {
          bodyEl.focus();
          bodyEl.setSelectionRange(bodyEl.value.length, bodyEl.value.length);
        }
      }
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  async function confirmDelete() {
    if (!current) return;
    const n = current;
    deleteOpen = false;
    clearTimeout(saveTimer);
    try {
      await api.Delete(n.kind, n.id);
      setCurrent(null);
      flash(`deleted ${keyOf(n)}`);
      await refreshList();
      loadLinkTargets();
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  function onSearchInput() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(refreshList, 200);
  }

  async function pickTag(t: string) {
    tagFilter = t;
    if (tab === 'tags' || tab === 'tasks') tab = 'page';
    await refreshList();
  }

  async function clearFilters() {
    tagFilter = '';
    query = '';
    await refreshList();
  }

  let totalCount = $state(0);

  function sortFor(t: string): string {
    return viewState.sort?.[t] || (SORTS[t]?.[0]?.[0] ?? '');
  }

  async function setSort(t: string, v: string) {
    viewState = { ...viewState, sort: { ...(viewState.sort ?? {}), [t]: v } };
    try { await api.SetSort(t, v); } catch (e) { flash(errMsg(e), true); }
    await refreshList();
  }

  async function loadViewState() {
    try { viewState = await api.ViewState(); } catch { /* keep defaults */ }
  }

  async function setSearchMode(m: SearchMode) {
    searchMode = m;
    savePref('searchMode', m);
    await refreshList();
  }

  function onSearchKey(e: KeyboardEvent) {
    if (e.key === 'Tab' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault();
      const i = SEARCH_MODES.indexOf(searchMode);
      setSearchMode(SEARCH_MODES[(i + (e.shiftKey ? -1 : 1) + SEARCH_MODES.length) % SEARCH_MODES.length]);
    } else if (e.key === 'Escape' && query) {
      e.preventDefault();
      e.stopPropagation();
      query = '';
      refreshList();
    }
  }

  async function applySaved(x: { mode: string; text: string }) {
    searchMode = (SEARCH_MODES.includes(x.mode as SearchMode) ? x.mode : 'name') as SearchMode;
    savePref('searchMode', searchMode);
    query = x.text;
    if (tab !== 'page' && tab !== 'journal') tab = 'page';
    mainView = 'note';
    await refreshList();
    flash(`search ${searchMode} “${x.text}”: ${items.length} note(s)`);
  }

  // Month headers for date-sorted, unsearched lists (the TUI shows the same).
  type Row = { header: string } | { note: Note };
  let rows = $derived.by((): Row[] => {
    const by = sortFor(tab);
    if (query.trim() || by === 'title' || (tab !== 'page' && tab !== 'journal')) return items.map((n) => ({ note: n }));
    const out: Row[] = [];
    let last = '';
    for (const n of items) {
      let label = 'Pinned';
      if (n.extra?.pinned !== true) {
        const d = by === 'id' ? new Date(n.id + 'T00:00:00') : new Date(by === 'created' ? n.created : n.updated);
        label = isNaN(d.getTime()) ? '' : d.toLocaleDateString('en-US', { month: 'long', year: 'numeric' });
      }
      if (label !== last) { out.push({ header: label }); last = label; }
      out.push({ note: n });
    }
    return out;
  });

  // Highlighting: name hits are character offsets into the title; text hits
  // are the search words.
  type Seg = { t: string; on: boolean };
  function hlChars(s: string, hits: number[] | null | undefined): Seg[] {
    if (!hits?.length) return [{ t: s, on: false }];
    const set = new Set(hits);
    const out: Seg[] = [];
    Array.from(s).forEach((ch, i) => {
      const on = set.has(i);
      if (out.length && out[out.length - 1].on === on) out[out.length - 1].t += ch;
      else out.push({ t: ch, on });
    });
    return out;
  }
  function hlTerms(s: string, q: string): Seg[] {
    const terms: string[] = [];
    q.replace(/"([^"]+)"|(\S+)/g, (_m, p: string, w: string) => { terms.push((p ?? w).toLowerCase()); return ''; });
    const lower = s.toLowerCase();
    const mark = new Array(s.length).fill(false);
    for (const t of terms.filter(Boolean)) {
      for (let i = lower.indexOf(t); i >= 0; i = lower.indexOf(t, i + 1)) for (let j = i; j < i + t.length; j++) mark[j] = true;
    }
    const out: Seg[] = [];
    for (let i = 0; i < s.length; i++) {
      if (out.length && out[out.length - 1].on === mark[i]) out[out.length - 1].t += s[i];
      else out.push({ t: s[i], on: mark[i] });
    }
    return out.length ? out : [{ t: s, on: false }];
  }

  function setView(m: ViewMode) {
    slashOpen = false;
    metaOpen = false;
    linkOpen = false;
    viewMode = m;
    savePref('viewMode', m);
  }

  // Apply [font] from config.toml: families go in front of the built-in stacks.
  const BODY_STACK = 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif';
  const MONO_STACK = 'ui-monospace, "JetBrains Mono", "Fira Code", Menlo, monospace';
  function applyFonts(c: Config | null) {
    const root = document.documentElement.style;
    const fam = (name: string | undefined, stack: string) =>
      name && name.trim() ? `"${name.trim().replace(/["\\]/g, '')}", ${stack}` : stack;
    root.setProperty('--font', fam(c?.font?.body, BODY_STACK));
    root.setProperty('--mono', fam(c?.font?.mono, MONO_STACK));
    root.setProperty('--size', `${c?.font?.size || 14}px`);
  }

  async function reloadConfig() {
    try {
      const c = await api.ReloadConfig();
      applyFonts(c);
      fmOpen = !c.frontmatter_collapsed;
      setStampDone(c.task_done_date !== false);
      if (settingsOpen) cfg = c;
      await loadUserSlash(true);
      await loadInfo();
      await refreshList();
      flash('config reloaded');
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  // Open whatever the start_page option asks for on launch.
  async function openStartPage() {
    let start = 'none';
    try {
      const c = await api.GetConfig();
      applyFonts(c);
      fmOpen = !c.frontmatter_collapsed;
      setStampDone(c.task_done_date !== false);
      start = c.start_page || 'none';
    } catch { return; }
    if (start === 'today') return openToday();
    // "last" is shared with the TUI (the recent file); older installs fall back to localStorage.
    const target = start === 'last' ? ((await api.LastNote().catch(() => '')) || loadPref('lastNote', '')) : start;
    const [kind, ...rest] = target.split('/');
    const id = rest.join('/');
    if ((kind !== 'page' && kind !== 'journal') || !id) return;
    try {
      const n = await api.Get(kind, id);
      if (!n) return;
      tab = kind as Tab;
      await refreshList();
      setCurrent(n);
    } catch {
      // A start note that was deleted or renamed just falls back to the welcome screen.
      if (start !== 'last') flash(`start page ${target} not found`, true);
    }
  }

  // Settings: start page picker state.
  let startPages: Note[] = $state([]);
  let startMode = $derived.by(() => {
    const v = (cfg as Config | null)?.start_page || 'none';
    return ['none', 'last', 'today'].includes(v) ? v : 'note';
  });

  async function setStartMode(mode: string) {
    if (mode !== 'note') return saveSetting('start_page', mode);
    const pick = current ? keyOf(current) : startPages[0] ? keyOf(startPages[0]) : '';
    if (!pick) { flash('no pages to choose from yet', true); return; }
    await saveSetting('start_page', pick);
  }

  async function openSettings() {
    try { startPages = (await api.List('page', '')) ?? []; } catch { startPages = []; }
    try {
      cfg = await api.GetConfig();
      settingsOpen = true;
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  async function saveSetting(key: string, value: string) {
    try {
      await api.SetOption(key, value);
      await loadInfo();
      cfg = await api.GetConfig();
      applyFonts(cfg);
      flash(`saved ${key}`);
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  async function chooseDir() {
    try {
      const dir = await api.ChooseNotesDir();
      if (dir) {
        await loadInfo();
        cfg = await api.GetConfig();
        setCurrent(null);
        await refreshList();
        flash(`notes folder: ${dir}`);
      }
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  async function loadInfo() {
    const info = await api.Info();
    notesDir = info.notesDir;
    configPath = info.configPath;
    version = info.version;
  }

  // Pick up edits made by the CLI/TUI when the window regains focus.
  async function onFocus() {
    await refreshList();
    loadLinkTargets();
    loadBacklinks();
    if (current && !dirty) {
      try {
        const fresh = await api.Get(current.kind, current.id);
        if (fresh && fresh.updated !== current.updated) setCurrent(fresh);
      } catch {
        setCurrent(null);
      }
    }
  }

  function toggleZoom() {
    zoomed = !zoomed;
  }

  // After "external" or "open-with", the note's file is polled while it
  // stays open here, so edits saved in the other program show up at once.
  // Unsaved edits here are never overwritten: the conflict bar appears.
  let extWatch: { key: string; stamp: string } | null = null;
  let extTimer: ReturnType<typeof setInterval> | undefined;
  let extChecking = false;
  // A function, so TypeScript doesn't narrow saveState across awaits.
  const saving = () => saveState === 'saving';

  function watchExternal(n: Note, stamp: string) {
    stopExternalWatch();
    extWatch = { key: keyOf(n), stamp };
    extTimer = setInterval(checkExternal, 1000);
  }

  function stopExternalWatch() {
    clearInterval(extTimer);
    extTimer = undefined;
    extWatch = null;
  }

  async function checkExternal() {
    const w = extWatch;
    if (!w || extChecking || saving()) return;
    if (!current || keyOf(current) !== w.key) { stopExternalWatch(); return; }
    extChecking = true;
    try {
      const { kind, id } = current;
      const stamp = await api.FileStamp(kind, id);
      if (!stamp || stamp === w.stamp) return;
      const fresh = await api.Get(kind, id);
      // A save started meanwhile, or the note was switched: try next tick.
      if (extWatch !== w || saving() || !current || keyOf(current) !== w.key) return;
      w.stamp = stamp;
      if (!fresh) return;
      if (dirty) {
        clearTimeout(saveTimer);
        conflict = true;
        saveState = 'error';
        return;
      }
      const sel = bodyEl ? { start: bodyEl.selectionStart, end: bodyEl.selectionEnd, top: bodyEl.scrollTop, focused: document.activeElement === bodyEl } : null;
      setCurrent(fresh);
      await tick();
      if (sel && bodyEl) {
        const len = bodyEl.value.length;
        bodyEl.setSelectionRange(Math.min(sel.start, len), Math.min(sel.end, len));
        bodyEl.scrollTop = sel.top;
        if (!sel.focused) bodyEl.blur();
      }
      flash(`reloaded ${keyOf(fresh)}: changed in the other editor`);
      refreshList();
      loadBacklinks();
    } catch {
      // The file may be mid-write; the next tick tries again.
    } finally {
      extChecking = false;
    }
  }

  async function openExternal() {
    if (!current) { flash('no note open', true); return; }
    await flushSave();
    try {
      const n = current;
      const stamp = await api.FileStamp(n.kind, n.id);
      await api.OpenExternal(n.kind, n.id);
      watchExternal(n, stamp);
      flash(`opened ${keyOf(n)} · watching for changes`);
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  async function openWith(r: CmdResult) {
    if (!current) { flash('no note open', true); return; }
    const prog = r.args?.[0] ?? '';
    await flushSave();
    try {
      const n = current;
      const stamp = await api.FileStamp(n.kind, n.id);
      await api.OpenWith(n.kind, n.id, prog);
      watchExternal(n, stamp);
      flash(`opened ${keyOf(n)} in ${prog.replace(/\s*&$/, '')} · watching for changes`);
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  async function openTerminal() {
    try {
      await api.OpenTerminal(current?.kind ?? '', current?.id ?? '');
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  // The command catalog, argument parsing and completion live in Go
  // (internal/commands, shared with the TUI). This side draws the palette,
  // applies results, and performs the UI-only commands.
  const cmdKeys: Record<string, string> = {
    zoom: `${modLabel}+Shift+Z`,
    external: `${modLabel}+Shift+O`,
    tasks: `${modLabel}+Shift+T`,
    journal: `${modLabel}+J`,
    help: 'F1',
  };
  const uiActions: Record<string, (r: CmdResult) => void | Promise<void>> = {
    zoom: toggleZoom,
    external: openExternal,
    terminal: openTerminal,
    'open-with': openWith,
    tasks: (r) => {
      for (const a of (r.args ?? []).map((x) => x.toLowerCase())) {
        if (a === 'open' || a === 'done' || a === 'all') setTaskFilter(a);
        else if (a === 'any' || a === 'overdue' || a === 'week') setTaskWhen(a);
      }
      return showTasks();
    },
    reload: reloadConfig,
    help: () => { helpOpen = true; },
    delete: () => { if (current) deleteOpen = true; },
    frontmatter: openFrontmatter,
    search: (r) => applySaved({ mode: r.args?.[0] ?? 'name', text: r.args?.[1] ?? '' }),
  };

  function usage(c: CmdSpec): string {
    return [c.name, ...(c.args ?? []).map((a) => {
      const n = a.name + (a.rest || a.many ? '…' : '');
      return a.optional ? `[${n}]` : `<${n}>`;
    })].join(' ');
  }

  let cmdItems = $derived(cmdComp?.items ?? []);

  // Pick mode (links, backlinks): the palette lists notes; typing filters them.
  let cmdPickItems = $derived.by(() => {
    if (!cmdPick) return [];
    const q = cmdInput.trim().toLowerCase();
    return q ? cmdPick.items.filter((c) => c.title.toLowerCase().includes(q) || c.id.includes(q)) : cmdPick.items;
  });

  // Ghost completion: the rest of the selected suggestion, when it extends what was typed.
  let cmdGhost = $derived.by(() => {
    const c = cmdComp;
    const s = cmdItems[cmdSel];
    if (cmdPick || !c || !s || !c.fragment || cmdInput !== c.keep + c.fragment) return '';
    const v = s.value.trimEnd();
    return v.length > c.fragment.length && v.slice(0, c.fragment.length).toLowerCase() === c.fragment.toLowerCase()
      ? v.slice(c.fragment.length) : '';
  });

  let cmdSeq = 0;
  async function refreshCompletion() {
    const seq = ++cmdSeq;
    const input = cmdInput;
    try {
      const c = await api.CompleteCommand(input, current?.kind ?? '', current?.id ?? '');
      if (seq !== cmdSeq) return;
      cmdComp = c;
      cmdSel = 0;
      cmdMoved = false;
    } catch (e) {
      if (seq === cmdSeq) cmdErr = errMsg(e);
    }
  }

  async function setCmdInput(v: string) {
    cmdInput = v;
    cmdErr = '';
    await refreshCompletion();
    await tick();
    cmdEl?.setSelectionRange(v.length, v.length);
  }

  function onCmdInput() {
    cmdHistIdx = -1;
    cmdErr = '';
    if (cmdPick) { cmdSel = 0; return; }
    refreshCompletion();
  }

  // Quick open (Ctrl+P): the palette as a jump-to-note box, ":open " with
  // fuzzy name matching and recent notes first; Enter takes the top match.
  let cmdQuick = $state(false);
  async function openQuickOpen() {
    await openPalette();
    cmdQuick = true;
    await setCmdInput('open ');
  }

  async function openPalette() {
    cmdQuick = false;
    cmdPick = null;
    cmdErr = '';
    cmdHistIdx = -1;
    cmdOpen = true;
    try { cmdHist = (await api.CommandHistory()) ?? []; } catch { cmdHist = []; }
    await setCmdInput('');
    cmdEl?.focus();
  }

  async function closePalette() {
    cmdOpen = false;
    cmdPick = null;
    await tick();
    bodyEl?.focus();
  }

  function accept(i: number) {
    const s = cmdItems[i];
    if (s && cmdComp) setCmdInput(cmdComp.keep + s.value);
  }

  async function runCommand() {
    if (cmdPick) {
      const c = cmdPickItems[cmdSel];
      if (c) { await closePalette(); await openChoice(c); }
      return;
    }
    let line = cmdInput;
    const c = cmdComp;
    if (c && cmdItems.length && (cmdMoved || (c.phase === 'command' && !c.exact) || (cmdQuick && c.phase === 'arg'))) line = c.keep + cmdItems[cmdSel].value;
    line = line.trim();
    if (!line) { await closePalette(); return; }
    if (dirty && conflict) { cmdErr = 'this note has a save conflict: reload or save it first'; return; }
    await flushSave();
    const spec = cmdCatalog.find((c) => line.toLowerCase() === c.name || line.toLowerCase().startsWith(c.name + ' '));
    if (spec?.long) { await closePalette(); flash(`${spec.name}…`); }
    let r: CmdResult;
    try {
      r = await api.RunCommand(line, current?.kind ?? '', current?.id ?? '', searchMode, query);
    } catch (e) {
      if (spec?.long) { flash(errMsg(e), true); return; }
      // Keep the palette open with the line, so it can be fixed.
      cmdInput = line;
      cmdErr = errMsg(e);
      return;
    }
    if (spec?.long && r.pickTitle) cmdOpen = true;
    await applyResult(r);
  }

  async function applyResult(r: CmdResult) {
    if (r.command?.startsWith('search')) await loadViewState();
    if (r.pickTitle) {
      if (r.pick?.length) {
        cmdPick = { title: r.pickTitle, items: r.pick };
        cmdInput = '';
        cmdSel = 0;
        cmdErr = '';
        return;
      }
      await closePalette();
      flash(r.status);
      return;
    }
    await closePalette();
    if (r.clipboard) {
      try { await Clipboard.SetText(r.clipboard); } catch { await navigator.clipboard?.writeText(r.clipboard); }
    }
    const act = r.ui ? uiActions[r.ui] : undefined;
    if (act) { await act(r); return; }
    if (r.note) {
      const want: Tab = r.note.kind === 'journal' ? 'journal' : 'page';
      if (tab !== want) tab = want;
      mainView = 'note';
      await refreshList();
      if (!items.some((n) => keyOf(n) === keyOf(r.note!)) && (tagFilter || query)) {
        tagFilter = '';
        query = '';
        await refreshList();
      }
      setCurrent(r.note);
      if (r.edit) { await tick(); bodyEl?.focus(); }
    } else if (r.changed) {
      await refreshList();
      await reloadCurrent();
    }
    if (r.changed) loadLinkTargets();
    if (r.status) flash(r.status);
  }

  async function openChoice(c: CmdChoice) {
    try {
      if (!c.exists) await api.EnsureNote(c.kind, c.id, c.title);
      const want: Tab = c.kind === 'journal' ? 'journal' : 'page';
      if (tab !== want) { tab = want; await refreshList(); }
      await open(c.kind, c.id);
      if (!c.exists) { await refreshList(); loadLinkTargets(); flash(`created ${c.kind}/${c.id}`); }
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  function moveSel(d: number) {
    const n = cmdPick ? cmdPickItems.length : cmdItems.length;
    if (n) { cmdSel = (cmdSel + d + n) % n; cmdMoved = true; }
  }

  function histStep(d: number): boolean {
    if (cmdPick) return false;
    const next = cmdHistIdx + d;
    if (d > 0 && !(cmdInput === '' || cmdHistIdx >= 0)) return false;
    if (d < 0 && cmdHistIdx < 0) return false;
    if (next >= cmdHist.length) return true;
    cmdHistIdx = next;
    setCmdInput(next < 0 ? '' : cmdHist[next]);
    return true;
  }

  function onCmdKey(e: KeyboardEvent) {
    const mod = e.ctrlKey || e.metaKey;
    if (e.key === 'Escape') {
      e.preventDefault(); e.stopPropagation(); closePalette();
    } else if (e.key === 'Enter') {
      e.preventDefault(); runCommand();
    } else if (e.key === 'Tab') {
      e.preventDefault();
      if (cmdPick) { moveSel(e.shiftKey ? -1 : 1); return; }
      const s = cmdItems[cmdSel];
      if (!s || !cmdComp) return;
      cmdHistIdx = -1;
      if (cmdComp.fragment === s.value) {
        // Already complete: cycle to the next/previous match.
        moveSel(e.shiftKey ? -1 : 1);
        cmdInput = cmdComp.keep + cmdItems[cmdSel].value;
      } else {
        accept(cmdSel);
      }
    } else if (e.key === 'ArrowDown' || (mod && e.key.toLowerCase() === 'n')) {
      e.preventDefault(); e.stopPropagation();
      if (!histStep(-1)) moveSel(1);
    } else if (e.key === 'ArrowUp' || (mod && e.key.toLowerCase() === 'p')) {
      e.preventDefault(); e.stopPropagation();
      if (!histStep(1)) moveSel(-1);
    } else if (mod) {
      // Keep global shortcuts (Ctrl+S, Ctrl+K, …) from firing under the palette.
      e.stopPropagation();
    }
  }

  // ---- frontmatter editor (":frontmatter") ----
  async function openFrontmatter() {
    if (!current) { flash('no note open', true); return; }
    await flushSave();
    try {
      const d = await api.FrontmatterText(current.kind, current.id);
      fmEditText = d.yaml;
      fmEditInferred = d.inferred;
      fmEditErr = '';
      fmEditOpen = true;
      await tick();
      fmEditEl?.focus();
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  async function saveFrontmatter() {
    if (!current) return;
    try {
      const r = await api.ApplyFrontmatter(current.kind, current.id, fmEditText);
      fmEditOpen = false;
      if (r?.note) {
        const renamed = keyOf(r.note) !== keyOf(current);
        setCurrent(r.note);
        const n = r.updated?.length ?? 0;
        flash(renamed ? `frontmatter saved; renamed to ${keyOf(r.note)}${n ? ` · updated links in ${n} note(s)` : ''}` : 'frontmatter saved');
      }
      await refreshList();
      loadLinkTargets();
      await tick();
      bodyEl?.focus();
    } catch (e) {
      fmEditErr = errMsg(e);
    }
  }

  function onFmEditKey(e: KeyboardEvent) {
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); fmEditOpen = false; bodyEl?.focus(); }
    else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); e.stopPropagation(); saveFrontmatter(); }
    else if (e.ctrlKey || e.metaKey) e.stopPropagation();
  }

  // ---- tasks across notes ----
  function taskVisible(t: Task): boolean {
    if (!(taskFilter === 'all' || (taskFilter === 'done' ? t.done : !t.done))) return false;
    if (taskWhen === 'any') return true;
    const due = t.meta?.due ?? '';
    if (!due) return false;
    if (taskWhen === 'overdue') return dateState(due) === 'past';
    const week = new Date();
    week.setDate(week.getDate() + 7);
    return due <= today(week);
  }
  const PRI_RANK: Record<string, number> = { high: 3, med: 2, low: 1 };
  // Due date (undated last), then priority, like Go's notes.SortTasks.
  function byDue(a: Task, b: Task): number {
    const da = a.meta?.due ?? '', db = b.meta?.due ?? '';
    if (da !== db) return !da ? 1 : !db ? -1 : da < db ? -1 : 1;
    return (PRI_RANK[b.meta?.pri ?? ''] ?? 0) - (PRI_RANK[a.meta?.pri ?? ''] ?? 0);
  }
  function chipsOf(t: Task): string {
    const m = t.meta ?? {};
    const pairs = Object.keys(m).map((key) => ({ key, value: m[key] ?? '' }));
    return pairs.length ? metaChips(pairs) : '';
  }
  function notStarted(t: Task): boolean {
    return !t.done && dateState(t.meta?.start ?? '') === 'future';
  }
  function noteMatches(nt: NoteTasks, q: string): boolean {
    if (tagFilter && !(nt.note.tags ?? []).includes(tagFilter)) return false;
    q = q.trim().toLowerCase();
    if (!q) return true;
    return nt.note.title.toLowerCase().includes(q) || (nt.tasks ?? []).some((t) => t.text.toLowerCase().includes(q));
  }
  // Sidebar Tasks tab: notes with tasks, filtered by the sidebar search.
  let taskSidebar = $derived(taskNotes.filter((nt) => noteMatches(nt, query)));
  // All-tasks view: groups with their visible tasks.
  let taskGroups = $derived.by(() => {
    const q = taskQuery.trim().toLowerCase();
    return taskNotes
      .filter((nt) => noteMatches(nt, ''))
      .map((nt) => {
        const titleHit = !!q && nt.note.title.toLowerCase().includes(q);
        const tasks = (nt.tasks ?? []).filter((t) => taskVisible(t) && (!q || titleHit || t.text.toLowerCase().includes(q)));
        return { nt, tasks };
      })
      .filter((g) => g.tasks.length);
  });
  let taskCount = $derived(taskGroups.reduce((n, g) => n + g.tasks.length, 0));
  // Due-sorted view: every visible task in one list.
  let taskFlat = $derived(
    taskGroups
      .flatMap((g) => g.tasks.map((t) => ({ nt: g.nt, t })))
      .sort((a, b) => byDue(a.t, b.t) || (a.nt.note.path < b.nt.note.path ? -1 : a.nt.note.path > b.nt.note.path ? 1 : a.t.line - b.t.line))
  );
  function setTaskSort(v: 'note' | 'due') {
    taskSort = v;
    savePref('taskSort', v);
  }
  function setTaskWhen(v: 'any' | 'overdue' | 'week') {
    taskWhen = v;
    savePref('taskWhen', v);
  }

  function setTaskFilter(f: 'open' | 'done' | 'all') {
    taskFilter = f;
    savePref('taskFilter', f);
  }

  async function showTasks() {
    slashOpen = false;
    metaOpen = false;
    linkOpen = false;
    mainView = 'tasks';
    taskNotes = (await api.Tasks().catch(() => taskNotes)) ?? [];
  }

  function toggleTasksView() {
    if (mainView === 'tasks') mainView = 'note';
    else showTasks();
  }

  function recount(nt: NoteTasks) {
    const ts = nt.tasks ?? [];
    nt.done = ts.filter((t) => t.done).length;
    nt.open = ts.length - nt.done;
  }

  async function toggleTaskIn(nt: NoteTasks, t: Task) {
    const done = !t.done;
    if (current && keyOf(current) === keyOf(nt.note)) {
      // The note is open in the editor: change the draft so unsaved text isn't clobbered.
      const next = setTaskLine(draftBody, t.line, done, t.text);
      if (next === null) {
        flash('that task moved in the editor; list refreshed', true);
        await refreshList();
        return;
      }
      draftBody = next;
      onEdit();
      t.done = done;
      recount(nt);
      return;
    }
    try {
      await api.SetTask(nt.note.kind, nt.note.id, t.line, done);
      t.done = done;
      recount(nt);
    } catch (e) {
      flash(errMsg(e), true);
    }
    await refreshList();
  }

  // Set one metadata key on a task from the all-tasks view ('' removes it).
  async function setTaskMetaIn(nt: NoteTasks, t: Task, key: string, value: string) {
    if (current && keyOf(current) === keyOf(nt.note)) {
      const next = setTaskMetaLine(draftBody, t.line, key, value, t.text);
      if (next === null) {
        flash('that task moved in the editor; list refreshed', true);
        await refreshList();
        return;
      }
      draftBody = next;
      onEdit();
      const m = { ...(t.meta ?? {}) };
      if (value) m[key] = value;
      else delete m[key];
      t.meta = m;
      return;
    }
    try {
      await api.SetTaskMeta(nt.note.kind, nt.note.id, t.line, key, value);
      taskNotes = (await api.Tasks()) ?? [];
    } catch (e) {
      flash(errMsg(e), true);
    }
  }
  function cyclePri(nt: NoteTasks, t: Task) {
    const order = ['', 'high', 'med', 'low'];
    const next = order[(order.indexOf(t.meta?.pri ?? '') + 1) % order.length];
    setTaskMetaIn(nt, t, 'pri', next);
  }

  // Open a note and put the cursor at the start of a body line.
  async function openAtLine(kind: string, id: string, line: number) {
    await open(kind, id);
    await tick();
    if (!bodyEl) return;
    const lines = draftBody.split('\n');
    let off = 0;
    for (let i = 0; i < Math.min(line, lines.length); i++) off += lines[i].length + 1;
    bodyEl.focus();
    bodyEl.setSelectionRange(off, off);
    const y = caretXY(bodyEl, off).y;
    bodyEl.scrollTop = Math.max(0, y - bodyEl.clientHeight / 3);
  }

  function isTyping(t: EventTarget | null): boolean {
    const el = t as HTMLElement | null;
    if (!el) return false;
    return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName);
  }

  function onKey(e: KeyboardEvent) {
    const mod = e.ctrlKey || e.metaKey;
    if (e.key === 'Escape') {
      if (newOpen || deleteOpen || settingsOpen || helpOpen || cmdOpen || fmEditOpen) {
        newOpen = deleteOpen = settingsOpen = helpOpen = cmdOpen = fmEditOpen = false;
        e.preventDefault();
      } else if (mainView === 'tasks') {
        mainView = 'note';
        e.preventDefault();
      }
      return;
    }
    if (e.key === 'F1' || (e.key === '?' && !mod && !e.altKey && !isTyping(e.target))) {
      e.preventDefault();
      helpOpen = !helpOpen;
      return;
    }
    if (!mod) return;
    if (e.shiftKey) {
      const k = e.key.toLowerCase();
      if (k === 'p' || k === ':' || e.code === 'Semicolon') {
        e.preventDefault();
        if (cmdOpen) closePalette(); else openPalette();
        return;
      }
      if (k === 'o') {
        e.preventDefault();
        openExternal();
        return;
      }
      if (k === 't') {
        e.preventDefault();
        toggleTasksView();
        return;
      }
    }
    switch (e.key.toLowerCase()) {
      case 'p': e.preventDefault(); if (cmdOpen) closePalette(); else openQuickOpen(); break;
      case 's': e.preventDefault(); save(true); break;
      case 'n': e.preventDefault(); openNew(); break;
      case 'k':
      case 'f': e.preventDefault(); searchEl?.focus(); searchEl?.select(); break;
      case 'j': e.preventDefault(); openToday(); break;
      case '=':
      case '+': e.preventDefault(); setScale(uiScale + 0.1); break;
      case '-':
      case '_': e.preventDefault(); setScale(uiScale - 0.1); break;
      case '0': e.preventDefault(); setScale(1); break;
      case 'e': e.preventDefault(); setView(viewMode === 'edit' ? 'split' : viewMode === 'split' ? 'preview' : 'edit'); break;
    }
  }

  function setScale(v: number) {
    uiScale = Math.round(Math.min(2.5, Math.max(0.5, v)) * 10) / 10;
    savePref('uiScale', String(uiScale));
    flash(`text size ${Math.round(uiScale * 100)}%`);
  }

  // Replace text[from,to) in the editor, keeping native undo when possible.
  function replaceRange(from: number, to: number, text: string, caret: number) {
    if (!bodyEl) return;
    bodyEl.focus();
    bodyEl.setSelectionRange(from, to);
    // execCommand keeps the textarea undo stack and fires `input` (bind:value + onEdit).
    if (!document.execCommand('insertText', false, text)) {
      draftBody = draftBody.slice(0, from) + text + draftBody.slice(to);
      onEdit();
    }
    return tick().then(() => bodyEl?.setSelectionRange(caret, caret));
  }

  // The line around the caret: start, end, and caret column.
  function caretLine() {
    if (!bodyEl || bodyEl.selectionStart !== bodyEl.selectionEnd) return null;
    const pos = bodyEl.selectionStart;
    const v = bodyEl.value;
    const start = v.lastIndexOf('\n', pos - 1) + 1;
    let end = v.indexOf('\n', pos);
    if (end < 0) end = v.length;
    return { pos, start, end, text: v.slice(start, end), col: pos - start };
  }

  function taskifyLine() {
    const l = caretLine();
    if (!l) return;
    const out = makeTask(l.text);
    replaceRange(l.start, l.end, out, l.start + out.length);
  }

  // Re-check whether the caret sits after a "/word" and show/hide the menu.
  function updateSlash() {
    const l = caretLine();
    const q = l ? slashQuery(l.text.slice(0, l.col)) : null;
    if (!bodyEl || q === null || !matchSlash(q, allSlash).length) {
      slashOpen = false;
      return;
    }
    if (!slashOpen || q !== slashQ) slashSel = 0;
    slashQ = q;
    // Anchor the menu at the "/" so it doesn't slide while typing.
    slashPos = menuPos(l!.pos - q.length - 1);
    slashOpen = true;
    metaOpen = false;
    linkOpen = false;
  }

  // Re-check whether the caret is after "[[query" and show/hide the picker.
  function updateLink() {
    const l = caretLine();
    const lq = l && !slashOpen ? linkQuery(l.text.slice(0, l.col)) : null;
    if (!bodyEl || !lq) {
      linkOpen = false;
      return;
    }
    if (!linkOpen || lq.start !== linkQ.start) linkSel = 0;
    else if (lq.q !== linkQ.q) linkSel = 0;
    linkQ = lq;
    linkPos = menuPos(l!.start + lq.start);
    linkOpen = true;
    metaOpen = false;
  }

  function acceptLink(s: LinkSuggestion | undefined) {
    const l = caretLine();
    const q = linkQ;
    linkOpen = false;
    if (!s || !l) return;
    const r = applyLink(l.text, l.col, q.start, linkMarkdown(s.kind, s.id, linkText(s)));
    replaceRange(l.start, l.end, r.line, l.start + r.col);
  }

  // Keys while the link picker is open. Returns true when handled.
  function linkKey(e: KeyboardEvent): boolean {
    if (!linkOpen || !linkItems.length) return false;
    const mod = e.ctrlKey || e.metaKey;
    const n = linkItems.length;
    if (e.key === 'ArrowDown' || (mod && e.key.toLowerCase() === 'n')) linkSel = (linkSel + 1) % n;
    else if (e.key === 'ArrowUp' || (mod && e.key.toLowerCase() === 'p')) linkSel = (linkSel - 1 + n) % n;
    else if (e.key === 'Enter' || e.key === 'Tab') acceptLink(linkItems[linkSel]);
    else if (e.key === 'Escape') linkOpen = false;
    else return false;
    e.preventDefault();
    e.stopPropagation();
    return true;
  }

  // Open the note a link points at, creating it first if it's missing.
  async function followLink(kind: string, id: string, text: string) {
    await flushSave();
    try {
      if (!linkKeys.has(`${kind}/${id}`)) {
        await api.EnsureNote(kind, id, text);
        flash(`created ${kind}/${id}`);
        await loadLinkTargets();
        await refreshList();
      }
      await open(kind, id);
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  // Where to put a dropdown anchored at text offset `at` in the editor.
  function menuPos(at: number) {
    const ta = bodyEl!;
    const c = caretXY(ta, at);
    const top = ta.offsetTop + c.y - ta.scrollTop;
    const paneH = ta.offsetParent ? (ta.offsetParent as HTMLElement).clientHeight : ta.clientHeight;
    const above = top + c.lineHeight + 200 > paneH && top > 200;
    return {
      left: Math.max(4, Math.min(ta.offsetLeft + c.x - ta.scrollLeft, ta.offsetLeft + ta.clientWidth - 260)),
      top: above ? top : top + c.lineHeight,
      above,
    };
  }

  // Re-check whether the caret is inside a task's {…} block and show/hide
  // the key/value suggestions. The slash menu takes precedence.
  function updateMeta() {
    const l = caretLine();
    const ctx = l && !slashOpen && !linkOpen ? metaContext(l.text, l.col) : null;
    if (!bodyEl || !ctx || !suggest(ctx, metaStats).length) {
      metaOpen = false;
      metaCtx = null;
      return;
    }
    const same = metaOpen && metaCtx && metaCtx.kind === ctx.kind && metaCtx.from === ctx.from;
    if (!same) metaSel = 0;
    metaCtx = ctx;
    metaPos = menuPos(l!.start + ctx.from);
    metaOpen = true;
  }

  function acceptMeta(sug: Suggestion | undefined) {
    const l = caretLine();
    const ctx = metaCtx;
    metaOpen = false;
    if (!sug || !l || !ctx) return;
    const r = applySuggestion(l.text, ctx, sug);
    replaceRange(l.start, l.end, r.line, l.start + r.col)?.then(updateMeta);
  }

  // Put the caret in the line's {…} block (adding it, and the task box, if
  // needed) and open the suggestions. With key, the caret lands on its value.
  function openMetaBlock(key?: string) {
    const l = caretLine();
    if (!l) return;
    const r = ensureMetaBlock(l.text, key);
    replaceRange(l.start, l.end, r.line, l.start + r.col)?.then(updateMeta);
  }

  // Keys for the metadata menu and the "{" pairing. Returns true when handled.
  function metaKey(e: KeyboardEvent): boolean {
    const mod = e.ctrlKey || e.metaKey;
    if (e.key === ' ' && e.ctrlKey && !e.shiftKey && !e.altKey) {
      openMetaBlock();
    } else if (e.key === '{' && !mod && !e.altKey) {
      const l = caretLine();
      if (!l || !shouldPairBrace(l.text, l.col)) return false;
      replaceRange(l.pos, l.pos, '{}', l.pos + 1)?.then(updateMeta);
    } else if (!metaOpen || !metaItems.length) {
      return false;
    } else if (e.key === 'ArrowDown' || (mod && e.key.toLowerCase() === 'n')) {
      metaSel = (metaSel + 1) % metaItems.length;
    } else if (e.key === 'ArrowUp' || (mod && e.key.toLowerCase() === 'p')) {
      metaSel = (metaSel - 1 + metaItems.length) % metaItems.length;
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      acceptMeta(metaItems[metaSel]);
    } else if (e.key === 'Escape') {
      metaOpen = false;
    } else {
      return false;
    }
    e.preventDefault();
    e.stopPropagation();
    return true;
  }

  async function acceptSlash(cmd: SlashCommand | undefined) {
    slashOpen = false;
    const l = caretLine();
    if (!cmd || !l) return;
    if (!cmd.user) {
      const r = expandSlash(l.text, l.col, cmd);
      if (r) replaceRange(l.start, l.end, r.line, l.start + r.col)?.then(() => { if (cmd.meta) updateMeta(); });
      return;
    }
    const tok = slashToken(l.text, l.col);
    if (!tok || !current) return;
    const note = current;
    try {
      const res = await api.RunSlash(cmd.name, {
        line: tok.before + tok.after,
        before: tok.before,
        after: tok.after,
        body: draftBody,
        note: { kind: note.kind, id: note.id, title: draftTitle, tags: parseTags(draftTags), path: '' },
        notes_dir: '',
        config_dir: '',
      });
      // The script ran asynchronously: only apply it if nothing moved underneath.
      if (!current || keyOf(current) !== keyOf(note) || !bodyEl || bodyEl.value.slice(l.start, l.end) !== l.text) {
        flash(`/${cmd.name}: the line changed while it ran; skipped`, true);
        return;
      }
      const r = applyUserResult(l.text, l.col, res);
      if (r) replaceRange(l.start, l.end, r.line, l.start + r.col);
    } catch (e) {
      flash(`/${cmd.name}: ${errMsg(e)}`, true);
    }
  }

  // Keys while the slash menu is open. Returns true when handled.
  function slashKey(e: KeyboardEvent): boolean {
    if (!slashOpen) return false;
    const mod = e.ctrlKey || e.metaKey;
    const n = slashItems.length;
    const k = e.key.toLowerCase();
    if (e.key === 'ArrowDown' || (mod && k === 'n')) {
      slashSel = (slashSel + 1) % n;
    } else if (e.key === 'ArrowUp' || (mod && k === 'p')) {
      slashSel = (slashSel - 1 + n) % n;
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      acceptSlash(slashItems[slashSel]);
    } else if (e.key === 'Escape') {
      slashOpen = false;
    } else if (e.key === ' ' && !mod) {
      // Space expands only an exact name ("/today "); otherwise it's just a space.
      const exact = allSlash.find((c) => c.name === slashQ);
      if (!exact) { slashOpen = false; return false; }
      acceptSlash(exact);
    } else {
      return false;
    }
    e.preventDefault();
    e.stopPropagation();
    return true;
  }

  function onPreviewClick(e: MouseEvent) {
    const a = (e.target as HTMLElement).closest?.('a');
    if (a) {
      // Never let the webview navigate away from the app.
      e.preventDefault();
      const href = a.getAttribute('href') ?? '';
      if (a.dataset.kind && a.dataset.id) followLink(a.dataset.kind, a.dataset.id, a.textContent ?? '');
      else if (/^(https?:|mailto:)/i.test(href)) Browser.OpenURL(href).catch((err) => flash(errMsg(err), true));
      else if (href.startsWith('#')) document.getElementById(decodeURIComponent(href.slice(1)))?.scrollIntoView();
      else if (href) flash(`can't open ${href}`, true);
      return;
    }
    const el = e.target as HTMLElement;
    if (!(el instanceof HTMLInputElement) || !el.classList.contains('task')) return;
    e.preventDefault();
    const next = toggleTask(draftBody, Number(el.dataset.task));
    if (next === null) return;
    draftBody = next;
    onEdit();
  }

  function onBodyKey(e: KeyboardEvent) {
    if (slashKey(e)) return;
    if (linkKey(e)) return;
    if (metaKey(e)) return;
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && !e.shiftKey && !e.altKey) {
      e.preventDefault();
      e.stopPropagation();
      taskifyLine();
      return;
    }
    if (e.key === 'Tab' && bodyEl) {
      e.preventDefault();
      const { selectionStart: s, selectionEnd: end } = bodyEl;
      draftBody = draftBody.slice(0, s) + '  ' + draftBody.slice(end);
      tick().then(() => bodyEl?.setSelectionRange(s + 2, s + 2));
      onEdit();
    }
  }

  const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  // "Nov 03 2026", the TUI's format.
  function fmtDay(s: string): string {
    const d = new Date(s);
    return isNaN(d.getTime()) ? '' : `${MONTHS[d.getMonth()]} ${String(d.getDate()).padStart(2, '0')} ${d.getFullYear()}`;
  }

  onMount(() => {
    loadInfo().catch((e) => flash(errMsg(e), true));
    refreshList().then(openStartPage);
    loadUserSlash();
    loadMetaStats();
    loadLinkTargets();
    loadViewState().then(refreshList);
    api.Commands().then((c) => { cmdCatalog = (c ?? []).filter((x): x is CmdSpec => !!x); }).catch(() => {});
    window.addEventListener('focus', onFocus);
    // Ctrl+Shift+Z is bound in main.go (Wails swallows it on Linux).
    window.addEventListener('katnotes:toggle-zoom', toggleZoom);
    window.addEventListener('beforeunload', () => { if (dirty) save(); });
    return () => {
      stopExternalWatch();
      window.removeEventListener('focus', onFocus);
      window.removeEventListener('katnotes:toggle-zoom', toggleZoom);
    };
  });
</script>

<svelte:window onkeydown={onKey} />

<div class="app" class:zoomed>
  {#if !zoomed}
  <aside class="sidebar">
    <div class="brand">
      <span class="logo">🐾</span> KatNotes
      <button class="icon" title="Keyboard help (?)" onclick={() => (helpOpen = true)}>?</button>
      <button class="icon settings" title="Settings" onclick={openSettings}>⚙</button>
    </div>

    <div class="actions">
      <button class="primary" onclick={openNew} title="Ctrl+N">+ New {tab === 'journal' ? 'entry' : 'page'}</button>
      <button onclick={openToday} title="Ctrl+J">Today</button>
    </div>

    <div class="search-modes" role="radiogroup" aria-label="Search mode">
      {#each SEARCH_MODES as md (md)}
        <button role="radio" aria-checked={searchMode === md} class:active={searchMode === md} onclick={() => setSearchMode(md)}>
          {md === 'name' ? 'Name' : md === 'tags' ? 'Tags' : 'Text'}
        </button>
      {/each}
    </div>
    <input
      class="search"
      type="search"
      placeholder="Search {searchMode === 'name' ? 'names' : searchMode}…  (Ctrl+K · Tab: mode)"
      title={SEARCH_HINTS[searchMode]}
      bind:value={query}
      bind:this={searchEl}
      oninput={onSearchInput}
      onkeydown={onSearchKey}
    />

    <nav class="tabs">
      <button class:active={tab === 'page'} onclick={() => switchTab('page')}>Pages</button>
      <button class:active={tab === 'journal'} onclick={() => switchTab('journal')}>Journal</button>
      <button class:active={tab === 'tags'} onclick={() => switchTab('tags')}>Tags</button>
      <button class:active={tab === 'tasks'} onclick={() => switchTab('tasks')}>Tasks</button>
    </nav>
    {#if tab === 'page' || tab === 'journal'}
      <div class="list-bar">
        <span class="hint">{query.trim() ? `${items.length} of ${totalCount}` : `${items.length} note${items.length === 1 ? '' : 's'}`}</span>
        <label class="sort-pick">Sort
          <select value={sortFor(tab)} onchange={(e) => setSort(tab, (e.currentTarget as HTMLSelectElement).value)}>
            {#each SORTS[tab] as [v, label] (v)}<option value={v}>{label}</option>{/each}
          </select>
        </label>
      </div>
    {/if}

    {#if tagFilter || query}
      <div class="filters">
        {#if tagFilter}<span class="chip">#{tagFilter}</span>{/if}
        {#if query}<span class="chip">{searchMode}: “{query}”</span>{/if}
        <button class="link" onclick={clearFilters}>clear</button>
      </div>
    {/if}

    {#if tab === 'tasks'}
      <button class="all-tasks" class:active={mainView === 'tasks'} onclick={showTasks} title="{modLabel}+Shift+T">☑ Show all tasks</button>
    {/if}

    <ul class="list">
      {#if tab === 'tasks'}
        {#each taskSidebar as nt (keyOf(nt.note))}
          {@const firstOpen = (nt.tasks ?? []).find((t) => !t.done)}
          <li>
            <button
              class="item"
              class:active={mainView === 'note' && current && keyOf(current) === keyOf(nt.note)}
              onclick={() => open(nt.note.kind, nt.note.id)}
            >
              <span class="title">{nt.note.title}</span>
              <span class="meta">
                <span class="task-count" class:none-open={!nt.open}>{nt.open} open · {nt.done} done</span>
                {nt.note.kind === 'journal' ? nt.note.id : nt.note.kind}
              </span>
              {#if firstOpen}<span class="snippet">☐ {firstOpen.text}</span>{/if}
            </button>
          </li>
        {:else}
          <li class="empty">{query || tagFilter ? 'No matching notes with tasks.' : 'No tasks yet. Type /todo or press Ctrl+Enter on a line.'}</li>
        {/each}
      {:else if tab === 'tags'}
        {#each viewState.searches ?? [] as x (x.name)}
          <li>
            <button class="item saved" onclick={() => applySaved(x)}>
              <span class="title">🔎 {x.name}</span>
              <span class="meta">{x.mode}: {x.text}</span>
            </button>
          </li>
        {/each}
        {#each tags as t (t.tag)}
          <li>
            <button class="item" class:active={tagFilter === t.tag} onclick={() => pickTag(t.tag)}>
              <span class="title">#{t.tag}</span>
              <span class="meta">{t.count} note{t.count === 1 ? '' : 's'}</span>
            </button>
          </li>
        {:else}
          <li class="empty">No tags yet.</li>
        {/each}
      {:else}
        {#each rows as row, ri ('header' in row ? 'h:' + row.header + ri : keyOf(row.note))}
          {#if 'header' in row}
            <li class="group">{row.header}</li>
          {:else}
          {@const n = row.note}
          {@const m = matches[keyOf(n)]}
          <li>
            <button
              class="item"
              class:active={current && keyOf(current) === keyOf(n)}
              onclick={() => open(n.kind, n.id)}
            >
              <!-- Same rows as the TUI list: dates line, then tags; a search adds the hit. -->
              <span class="title">{#if n.problem}<span class="warn" title={n.problem}>⚠</span>{/if}{#if n.extra?.pinned === true}<span class="pin" title="pinned">📌</span>{/if}{#each hlChars(n.title, m?.nameHits) as sg}{#if sg.on}<mark>{sg.t}</mark>{:else}{sg.t}{/if}{/each}</span>
              {#if n.kind === 'journal'}
                <span class="meta">{n.id}{#each n.tags ?? [] as t}<span class="tag">#{t}</span>{/each}</span>
              {:else}
                <span class="meta dates">{fmtDay(n.created)} <span class="sep">│</span> m: {fmtDay(n.updated)}</span>
                {#if n.tags?.length}<span class="meta">{#each n.tags as t}<span class="tag">#{t}</span>{/each}</span>{/if}
              {/if}
              {#if m && m.lines && m.lines.length}
                <span class="snippet">…{#each hlTerms(m.lines[0].text.trim(), searchMode === 'text' ? query : '') as sg}{#if sg.on}<mark>{sg.t}</mark>{:else}{sg.t}{/if}{/each}</span>
              {/if}
            </button>
          </li>
          {/if}
        {:else}
          <li class="empty">
            {query ? 'No matches.' : tab === 'journal' ? 'No journal entries yet. Click Today.' : 'No pages yet.'}
          </li>
        {/each}
      {/if}
    </ul>

    <footer class="dir" title={notesDir}>{notesDir}</footer>
  </aside>
  {/if}

  <main class="main">
    {#if mainView === 'tasks'}
      <section class="tasks-view">
        <header class="tasks-head">
          <h2>Tasks <span class="tasks-total">{taskCount}</span></h2>
          <div class="seg">
            <button class:active={taskFilter === 'open'} onclick={() => setTaskFilter('open')}>Open</button>
            <button class:active={taskFilter === 'done'} onclick={() => setTaskFilter('done')}>Done</button>
            <button class:active={taskFilter === 'all'} onclick={() => setTaskFilter('all')}>All</button>
          </div>
          <div class="seg" title="Due date">
            <button class:active={taskWhen === 'any'} onclick={() => setTaskWhen('any')}>Any date</button>
            <button class:active={taskWhen === 'overdue'} onclick={() => setTaskWhen('overdue')}>Overdue</button>
            <button class:active={taskWhen === 'week'} onclick={() => setTaskWhen('week')}>Next 7 days</button>
          </div>
          <div class="seg" title="Sort">
            <button class:active={taskSort === 'note'} onclick={() => setTaskSort('note')}>By note</button>
            <button class:active={taskSort === 'due'} onclick={() => setTaskSort('due')}>By due</button>
          </div>
          <input class="tasks-filter" type="search" placeholder="Filter tasks…" bind:value={taskQuery} />
          {#if tagFilter}<span class="chip">#{tagFilter}</span>{/if}
          <button class="link" onclick={() => (mainView = 'note')} title="Esc">Back to note</button>
        </header>
        {#snippet taskRow(nt: NoteTasks, t: Task, showNote: boolean)}
          <li class="task-row" class:done={t.done} class:not-started={notStarted(t)} style:padding-left="{showNote ? 0 : Math.min(t.indent, 16) * 6}px">
            <input type="checkbox" class="task" checked={t.done} onclick={(e) => { e.preventDefault(); toggleTaskIn(nt, t); }} aria-label="Toggle task" />
            <button class="task-text" onclick={() => openAtLine(nt.note.kind, nt.note.id, t.line)} title="Open at this line">
              {t.text || '(empty task)'}
              {#if chipsOf(t)}{@html chipsOf(t)}{/if}
              {#if showNote}<span class="task-note-meta"> · {nt.note.title}</span>{/if}
            </button>
            <span class="task-tools">
              <input type="date" class="task-due" value={t.meta?.due ?? ''} title="Due date" aria-label="Due date"
                onchange={(e) => setTaskMetaIn(nt, t, 'due', (e.currentTarget as HTMLInputElement).value)} />
              <button class="task-pri" onclick={() => cyclePri(nt, t)} title="Priority: none → high → med → low">!</button>
            </span>
          </li>
        {/snippet}
        <div class="tasks-body">
          {#if taskSort === 'due' && taskFlat.length}
            <ul class="task-flat">
              {#each taskFlat as r (keyOf(r.nt.note) + '#' + r.t.line)}
                {@render taskRow(r.nt, r.t, true)}
              {/each}
            </ul>
          {:else}
          {#each taskGroups as g (keyOf(g.nt.note))}
            <div class="task-group">
              <button class="task-note" onclick={() => open(g.nt.note.kind, g.nt.note.id)}>
                {g.nt.note.title}
                <span class="task-note-meta">{g.nt.note.kind === 'journal' ? g.nt.note.id : 'page'} · {g.nt.open} open · {g.nt.done} done</span>
              </button>
              <ul>
                {#each g.tasks.toSorted((a, b) => (taskSort === 'due' ? byDue(a, b) : 0)) as t (t.line)}
                  {@render taskRow(g.nt, t, false)}
                {/each}
              </ul>
            </div>
          {:else}
            <p class="empty">
              {taskNotes.length ? `No ${taskFilter === 'all' ? '' : taskFilter + ' '}tasks${taskQuery || tagFilter || taskWhen !== 'any' ? ' match' : ''}.` : 'No tasks yet. Type /todo or press Ctrl+Enter on a line.'}
            </p>
          {/each}
          {/if}
        </div>
      </section>
    {:else if current}
      <header class="note-head" class:bare={zoomed}>
        {#if !zoomed}
        <input
          class="title-input"
          bind:value={draftTitle}
          onblur={commitTitle}
          onkeydown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
        />
        <div class="head-row">
          <span class="kind">{current.kind}/{current.id}</span>
          <input
            class="tags-input"
            placeholder="tags, comma separated"
            bind:value={draftTags}
            oninput={onEdit}
          />
          <div class="seg">
            <button class:active={viewMode === 'edit'} onclick={() => setView('edit')}>Edit</button>
            <button class:active={viewMode === 'split'} onclick={() => setView('split')}>Split</button>
            <button class:active={viewMode === 'preview'} onclick={() => setView('preview')}>Preview</button>
          </div>
          <span class="save {saveState}">
            {saveState === 'saving' ? 'Saving…' : saveState === 'dirty' ? 'Unsaved' : saveState === 'error' ? 'Not saved' : 'Saved'}
          </span>
          <button class="danger" onclick={() => (deleteOpen = true)} title="Delete note">Delete</button>
        </div>
        {/if}
        {#if conflict}
          <div class="conflict">
            This note changed on disk (CLI, TUI or another editor).
            <button onclick={reloadCurrent}>Reload theirs</button>
            <button onclick={() => save(true)}>Keep mine</button>
          </div>
        {/if}
      </header>

      <section class="panes {viewMode}">
        {#if viewMode !== 'preview'}
          <div class="editor-col">
          <details class="frontmatter" open={fmOpen} ontoggle={(e) => { fmOpen = (e.currentTarget as HTMLDetailsElement).open; }}>
            <summary>Frontmatter <span class="fm-count">{fmRows.length} fields</span></summary>
            <table>
              <tbody>
                {#each fmRows as [k, v] (k)}
                  <tr><th scope="row">{k}</th><td class:empty-val={!v}>{v || '—'}</td></tr>
                {/each}
              </tbody>
            </table>
          </details>
          <textarea
            class="editor"
            bind:this={bodyEl}
            bind:value={draftBody}
            oninput={() => { onEdit(); updateSlash(); updateLink(); updateMeta(); }}
            onkeydown={onBodyKey}
            onkeyup={(e) => { if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) { updateSlash(); updateLink(); updateMeta(); } }}
            onclick={() => { updateSlash(); updateLink(); updateMeta(); }}
            onblur={() => { slashOpen = false; metaOpen = false; linkOpen = false; }}
            onscroll={() => { if (slashOpen) updateSlash(); if (linkOpen) updateLink(); if (metaOpen) updateMeta(); }}
            spellcheck="true"
          ></textarea>
          {#if slashOpen && slashItems.length}
            <EditorMenu
              items={slashItems.map((c) => ({ key: c.name, label: '/' + c.name, detail: c.desc(), badge: c.user ? 'lua' : undefined }))}
              sel={slashSel} pos={slashPos}
              onpick={(i) => { slashSel = i; acceptSlash(slashItems[i]); }}
              onhover={(i) => (slashSel = i)} />
          {/if}
          {#if linkOpen && linkItems.length}
            <EditorMenu wide
              items={linkItems.map((x) => ({ key: x.kind + '/' + x.id + (x.isNew ? '+' : ''), label: x.label, detail: x.detail, muted: x.isNew }))}
              sel={linkSel} pos={linkPos}
              onpick={(i) => { linkSel = i; acceptLink(linkItems[i]); }}
              onhover={(i) => (linkSel = i)} />
          {/if}
          {#if metaOpen && metaItems.length}
            <EditorMenu
              items={metaItems.map((x) => ({ key: x.label + x.insert, label: x.label + (x.then ? ':' : ''), detail: x.detail }))}
              sel={metaSel} pos={metaPos}
              onpick={(i) => { metaSel = i; acceptMeta(metaItems[i]); }}
              onhover={(i) => (metaSel = i)} />
          {/if}
          </div>
        {/if}
        {#if viewMode !== 'edit'}
          <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
          <article class="preview" onclick={onPreviewClick}>{@html preview}</article>
        {/if}
      </section>
      {#if backlinks.length && !zoomed}
        <details class="backlinks" open>
          <summary>Linked from <span class="backlinks-count">{backlinks.length}</span></summary>
          <ul>
            {#each backlinks as b (keyOf(b.note))}
              <li>
                <button class="backlink-note" onclick={() => open(b.note.kind, b.note.id)}>
                  {b.note.title}<span class="task-note-meta"> · {b.note.kind === 'journal' ? b.note.id : 'page'}</span>
                </button>
                {#each b.lines ?? [] as l (l.line)}
                  <button class="backlink-line" onclick={() => openAtLine(b.note.kind, b.note.id, l.line - 1)} title="Open at this line">{plainLinks(l.text)}</button>
                {/each}
              </li>
            {/each}
          </ul>
        </details>
      {/if}
    {:else}
      <div class="welcome">
        <h1>KatNotes</h1>
        <p>Select a note, or start something new.</p>
        <div class="actions">
          <button class="primary" onclick={openNew}>New page</button>
          <button onclick={openToday}>Today's journal</button>
          <button onclick={showTasks}>Tasks</button>
        </div>
        <p class="keys">Ctrl+N new · Ctrl+J today · Ctrl+K search · Ctrl+E edit/split/preview · Ctrl+Shift+P commands · Ctrl+Shift+T tasks · ? help</p>
      </div>
    {/if}
    {#if zoomed}
      <button class="unzoom" title="Exit zoom ({modLabel}+Shift+Z)" onclick={() => (zoomed = false)}>⤡</button>
    {/if}
    {#if status}
      <div class="status" class:err={statusErr}>
        {status}
        <button class="link" onclick={() => (status = '')}>×</button>
      </div>
    {/if}
  </main>
</div>

{#if newOpen}
  <div class="backdrop" role="presentation" onclick={() => (newOpen = false)}>
    <form class="modal" onsubmit={(e) => { e.preventDefault(); createNote(); }} onclick={(e) => e.stopPropagation()} role="presentation">
      <h2>{tab === 'journal' ? 'New journal entry' : 'New page'}</h2>
      <input
        bind:this={newEl}
        bind:value={newTitle}
        placeholder={tab === 'journal' ? 'YYYY-MM-DD (empty for today)' : 'Title'}
      />
      {#if tab !== 'journal' && templates.length}
        <label class="tmpl-pick">Template
          <select bind:value={newTemplate}>
            <option value="">Blank page</option>
            {#each templates as t (t)}<option value={t}>{t}</option>{/each}
          </select>
        </label>
      {/if}
      {#if tagFilter}<p class="hint">Will be tagged #{tagFilter}</p>{/if}
      <div class="modal-actions">
        <button type="button" onclick={() => (newOpen = false)}>Cancel</button>
        <button type="submit" class="primary">Create</button>
      </div>
    </form>
  </div>
{/if}

{#if deleteOpen && current}
  <div class="backdrop" role="presentation" onclick={() => (deleteOpen = false)}>
    <div class="modal" role="presentation" onclick={(e) => e.stopPropagation()}>
      <h2>Delete “{current.title}”?</h2>
      <p class="hint">{current.path}</p>
      <div class="modal-actions">
        <button onclick={() => (deleteOpen = false)}>Cancel</button>
        <button class="danger" onclick={confirmDelete}>Delete</button>
      </div>
    </div>
  </div>
{/if}

{#if cmdOpen}
  <div class="backdrop palette-backdrop" role="presentation" onclick={closePalette}>
    <div class="modal palette" role="dialog" aria-label="Command palette" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
      {#if cmdPick}<div class="cmd-pick-title">{cmdPick.title}</div>{/if}
      <div class="cmd-line">
        <span class="cmd-prompt">{cmdPick ? '›' : ':'}</span>
        <div class="cmd-field">
          <span class="cmd-ghost" aria-hidden="true"><span class="cmd-typed">{cmdInput}</span>{cmdGhost}</span>
          <input
            bind:this={cmdEl}
            bind:value={cmdInput}
            oninput={onCmdInput}
            onkeydown={onCmdKey}
            spellcheck="false"
            autocomplete="off"
            aria-label={cmdPick ? 'Filter notes' : 'Command'}
            placeholder={cmdPick ? 'filter…' : ''}
          />
        </div>
      </div>
      {#if !cmdPick && cmdComp && (cmdComp.phase !== 'command' || cmdComp.exact)}
        <div class="cmd-usage"><code>{cmdComp.usage}</code> {cmdComp.desc}</div>
      {/if}
      {#if cmdErr}<div class="cmd-err">{cmdErr}</div>{/if}
      <ul class="cmd-list">
        {#if cmdPick}
          {#each cmdPickItems as c, i (c.kind + '/' + c.id)}
            <li>
              <button class="cmd-item" class:active={i === cmdSel} class:muted={!c.exists} onmouseenter={() => (cmdSel = i)} onclick={() => { cmdSel = i; runCommand(); }}>
                <span class="cmd-name">{c.title}</span>
                <span class="cmd-desc">{c.detail}</span>
              </button>
            </li>
          {:else}
            <li class="empty">No matching note.</li>
          {/each}
        {:else}
          {#each cmdItems as c, i (c.value + (c.recent ? ' r' : ''))}
            <li>
              <button class="cmd-item" class:active={i === cmdSel} onmouseenter={() => (cmdSel = i)} onclick={() => {
                if (cmdComp?.phase === 'command' && !c.value.endsWith(' ')) { cmdSel = i; cmdMoved = true; runCommand(); }
                else { accept(i); cmdEl?.focus(); }
              }}>
                <span class="cmd-name">{#if c.recent}<span class="cmd-recent" title="recent">↺</span>{/if}{c.label}</span>
                <span class="cmd-desc">{c.detail}</span>
                {#if cmdComp?.phase === 'command' && !c.recent && cmdKeys[c.value.trim()]}<kbd>{cmdKeys[c.value.trim()]}</kbd>{/if}
              </button>
            </li>
          {:else}
            <li class="empty">
              {#if cmdComp?.phase === 'arg'}&lt;{cmdComp.arg}&gt;: type a value, Enter runs it
              {:else if cmdComp?.phase === 'none'}Enter runs it
              {:else}No matching command.{/if}
            </li>
          {/each}
        {/if}
      </ul>
      <div class="cmd-foot hint">
        {#if cmdPick}↑↓ select · Enter open · Esc close
        {:else if cmdHist.length && !cmdInput}↑ history · Tab complete · Enter run · Esc close
        {:else}Tab complete · ↑↓ select · Enter run · Esc close{/if}
      </div>
    </div>
  </div>
{/if}

{#if fmEditOpen && current}
  <div class="backdrop" role="presentation" onclick={() => (fmEditOpen = false)}>
    <div class="modal fm-edit" role="dialog" aria-label="Edit frontmatter" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={onFmEditKey}>
      <h2>Frontmatter · {current.title}</h2>
      {#if fmEditInferred}
        <p class="hint">This note has no frontmatter yet. These values were worked out from the file (first heading, file dates); saving adds them to it.</p>
      {/if}
      <textarea bind:this={fmEditEl} bind:value={fmEditText} spellcheck="false" rows="12"></textarea>
      <p class="hint">YAML. <code>tags: [a, b]</code> · <code>created: 2026-01-31</code> · <code>updated</code> is set when you save · a new title renames the note.</p>
      {#if fmEditErr}<div class="cmd-err">{fmEditErr}</div>{/if}
      <div class="modal-actions">
        <button onclick={() => (fmEditOpen = false)}>Cancel</button>
        <button class="primary" onclick={saveFrontmatter} title="{modLabel}+Enter">Save</button>
      </div>
    </div>
  </div>
{/if}

{#if helpOpen}
  <div class="backdrop" role="presentation" onclick={() => (helpOpen = false)}>
    <div class="modal help" role="dialog" aria-label="Keyboard help" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={() => {}}>
      <header class="help-head">
        <div>
          <div class="help-title">Keyboard help</div>
          <div class="help-sub">KatNotes desktop</div>
        </div>
        <span class="version-pill">katnotes-gui {version || 'dev'}</span>
      </header>
      <div class="help-body">
        {#each helpSections as sec}
          <h3>{sec.title}</h3>
          <dl>
            {#each sec.keys as [k, d]}
              <dt><kbd>{k}</kbd></dt>
              <dd>{d}</dd>
            {/each}
          </dl>
        {/each}
        <h3>Command palette</h3>
        <dl>
          {#each cmdCatalog as c (c.name)}
            <dt><kbd>:{usage(c)}</kbd></dt>
            <dd>{c.desc}{#if cmdKeys[c.name]} <span class="hint">({cmdKeys[c.name]})</span>{/if}</dd>
          {/each}
        </dl>
      </div>
      <footer class="help-foot">
        <span class="hint">Esc or ? to close</span>
        <button class="primary" onclick={() => (helpOpen = false)}>Close</button>
      </footer>
    </div>
  </div>
{/if}

{#if settingsOpen && cfg}
  <div class="backdrop" role="presentation" onclick={() => (settingsOpen = false)}>
    <div class="modal wide" role="presentation" onclick={(e) => e.stopPropagation()}>
      <h2>Settings</h2>
      <label>Notes folder
        <div class="row">
          <input value={cfg.notes_dir} readonly />
          <button onclick={chooseDir}>Change…</button>
        </div>
      </label>
      <label>External editor (CLI/TUI)
        <input value={cfg.editor} placeholder="$VISUAL / $EDITOR" onchange={(e) => saveSetting('editor', (e.target as HTMLInputElement).value)} />
      </label>
      <label>Open on launch
        <div class="row">
          <select value={startMode} onchange={(e) => setStartMode((e.target as HTMLSelectElement).value)}>
            <option value="none">Welcome screen</option>
            <option value="last">Last opened note</option>
            <option value="today">Today's journal entry</option>
            <option value="note">A specific note…</option>
          </select>
          {#if startMode === 'note'}
            <select value={cfg.start_page} onchange={(e) => saveSetting('start_page', (e.target as HTMLSelectElement).value)}>
              {#if current && current.kind === 'journal'}
                <option value={keyOf(current)}>{current.title} (journal)</option>
              {/if}
              {#if !startPages.some((p) => keyOf(p) === cfg?.start_page) && !(current && keyOf(current) === cfg.start_page)}
                <option value={cfg.start_page}>{cfg.start_page}</option>
              {/if}
              {#each startPages as p (keyOf(p))}
                <option value={keyOf(p)}>{p.title}</option>
              {/each}
            </select>
          {/if}
        </div>
      </label>
      <div class="row font-row">
        <label>Body font
          <input value={cfg.font?.body ?? ''} placeholder="system default" onchange={(e) => saveSetting('font.body', (e.target as HTMLInputElement).value)} />
        </label>
        <label>Mono font
          <input value={cfg.font?.mono ?? ''} placeholder="system monospace" onchange={(e) => saveSetting('font.mono', (e.target as HTMLInputElement).value)} />
        </label>
        <label class="size">Size (px)
          <input type="number" min="8" max="40" value={cfg.font?.size ?? 14} onchange={(e) => saveSetting('font.size', (e.target as HTMLInputElement).value)} />
        </label>
      </div>
      <label class="check">
        <input type="checkbox" checked={cfg.frontmatter_collapsed}
          onchange={async (e) => { const v = (e.target as HTMLInputElement).checked; await saveSetting('frontmatter_collapsed', String(v)); fmOpen = !v; }} />
        Start with the frontmatter table collapsed
      </label>
      <label>Open notes with (GUI)
        <input value={cfg.open_with} placeholder="system default (xdg-open)" onchange={(e) => saveSetting('open_with', (e.target as HTMLInputElement).value)} />
      </label>
      <label>Journal title date format (Go layout)
        <input value={cfg.date_format} onchange={(e) => saveSetting('date_format', (e.target as HTMLInputElement).value)} />
      </label>
      <label>Journal template
        <textarea rows="4" value={cfg.journal_template} onchange={(e) => saveSetting('journal_template', (e.target as HTMLTextAreaElement).value)}></textarea>
      </label>
      <p class="hint">Config: {configPath} · katnotes-gui {version}</p>
      <div class="modal-actions">
        <button class="primary" onclick={() => (settingsOpen = false)}>Done</button>
      </div>
    </div>
  </div>
{/if}
