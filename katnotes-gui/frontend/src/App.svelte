<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import { checkboxHTML, decorateTasks, toggleTask, makeTask, setTaskLine, setTaskMetaLine, metaChips, setStampDone, dateState, today, ensureMetaBlock } from './tasks';
  import { metaContext, suggest, applySuggestion, shouldPairBrace, type MetaContext, type MetaStat, type Suggestion } from './metacomplete';
  import { SLASH, matchSlash, mergeSlash, slashQuery, slashToken, expandSlash, applyUserResult, caretXY, rank, type SlashCommand } from './slash';
  import { NotesService as api } from '../bindings/github.com/devinthemtn/katnotes/katnotes-gui';
  import type { Note, Match, TagCount, NoteTasks, Task } from '../bindings/github.com/devinthemtn/katnotes/internal/notes';
  import type { Config } from '../bindings/github.com/devinthemtn/katnotes/internal/config';

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
  let deleteOpen = $state(false);
  let settingsOpen = $state(false);
  let helpOpen = $state(false);
  let zoomed = $state(false);

  // command palette
  let cmdOpen = $state(false);
  let cmdInput = $state('');
  let cmdSel = $state(0);
  let cmdEl: HTMLInputElement | undefined = $state();

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
      ['{', 'On a task line: start a {key:value} block; suggests due, pri, start and your keys, then values'],
      ['Ctrl+Space', 'On a task line: open the {…} suggestions (adds the block if needed)'],
      ['Click a box', 'In the preview: check or uncheck a task'],
      ['Tab', 'In the editor: indent two spaces'],
    ]},
    { title: 'Finding', keys: [
      [`${modLabel}+K / ${modLabel}+F`, 'Focus search'],
      ['Tags tab', 'Click a tag to filter Pages and Journal'],
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
    DOMPurify.sanitize(decorateTasks(marked.parse(draftBody, { async: false, gfm: true, breaks: false }) as string))
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
      if (query.trim()) {
        const res = (await api.Search(query, tab)) ?? [];
        const m: Record<string, Match> = {};
        const list: Note[] = [];
        for (const r of res) {
          if (tagFilter && !(r.note.tags ?? []).includes(tagFilter)) continue;
          m[keyOf(r.note)] = r;
          list.push(r.note);
        }
        matches = m;
        items = list;
      } else {
        matches = {};
        items = (await api.List(tab, tagFilter)) ?? [];
      }
    } catch (e) {
      flash(errMsg(e), true);
    }
  }

  function setCurrent(n: Note | null) {
    slashOpen = false;
    metaOpen = false;
    current = n;
    if (n) savePref('lastNote', keyOf(n));
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
    const target = current;
    saveState = 'saving';
    try {
      const n = await api.Save(target.kind, target.id, parseTags(draftTags), draftBody, target.updated, force);
      if (n && current && keyOf(current) === keyOf(target)) {
        current = { ...n, body: draftBody };
        dirty = false;
        conflict = false;
        saveState = 'saved';
      }
      await refreshList();
      loadMetaStats();
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
      if (r) {
        setCurrent(r);
        flash(`renamed to ${keyOf(r)}`);
      }
      await refreshList();
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
    newOpen = true;
    await tick();
    newEl?.focus();
  }

  async function createNote() {
    const kind = tab === 'journal' ? 'journal' : 'page';
    if (kind === 'page' && !newTitle.trim()) return;
    await flushSave();
    try {
      const n = await api.Create(kind, newTitle.trim(), tagFilter ? [tagFilter] : []);
      newOpen = false;
      if (tab === 'tags' || tab === 'tasks') tab = 'page';
      mainView = 'note';
      await refreshList();
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

  function setView(m: ViewMode) {
    slashOpen = false;
    metaOpen = false;
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
    const target = start === 'last' ? loadPref('lastNote', '') : start;
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

  async function openExternal() {
    if (!current) { flash('no note open', true); return; }
    await flushSave();
    try {
      await api.OpenExternal(current.kind, current.id);
      flash(`opened ${keyOf(current)}`);
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

  type Command = { name: string; desc: string; keys?: string; run: () => void | Promise<void> };
  const commands: Command[] = [
    { name: 'zoom', desc: 'Toggle focus mode (hide sidebar and top bar)', keys: `${modLabel}+Shift+Z`, run: toggleZoom },
    { name: 'open', desc: 'Open the note in the default editor', keys: `${modLabel}+Shift+O`, run: openExternal },
    { name: 'terminal', desc: "Open a terminal in the note's folder", run: openTerminal },
    { name: 'tasks', desc: 'Show all tasks from every note', keys: `${modLabel}+Shift+T`, run: showTasks },
    { name: 'reload', desc: 'Reload config.toml and Lua / commands', run: reloadConfig },
  ];

  let cmdMatches = $derived(rank(commands, cmdInput, (c) => c.desc));

  // Ghost completion: the rest of the selected command's name, when it extends what was typed.
  let cmdGhost = $derived.by(() => {
    const c = cmdMatches[cmdSel];
    const q = cmdInput.trim().toLowerCase();
    return c && q && c.name.startsWith(q) ? c.name.slice(q.length) : '';
  });

  async function openPalette() {
    cmdInput = '';
    cmdSel = 0;
    cmdOpen = true;
    await tick();
    cmdEl?.focus();
  }

  async function closePalette() {
    cmdOpen = false;
    await tick();
    bodyEl?.focus();
  }

  async function runCommand() {
    const q = cmdInput.trim().toLowerCase();
    const c = commands.find((x) => x.name === q) ?? cmdMatches[cmdSel];
    if (!c) {
      cmdOpen = false;
      if (q) flash(`unknown command: ${q}`, true);
      return;
    }
    await closePalette();
    await c.run();
  }

  function moveSel(d: number) {
    const n = cmdMatches.length;
    if (n) cmdSel = (cmdSel + d + n) % n;
  }

  function onCmdKey(e: KeyboardEvent) {
    const mod = e.ctrlKey || e.metaKey;
    if (e.key === 'Escape') {
      e.preventDefault(); e.stopPropagation(); closePalette();
    } else if (e.key === 'Enter') {
      e.preventDefault(); runCommand();
    } else if (e.key === 'Tab') {
      e.preventDefault();
      const c = cmdMatches[cmdSel];
      if (c && cmdInput.trim().toLowerCase() !== c.name) {
        cmdInput = c.name;
      } else {
        // Already complete: cycle to the next/previous match.
        moveSel(e.shiftKey ? -1 : 1);
        if (cmdMatches[cmdSel]) cmdInput = cmdMatches[cmdSel].name;
      }
    } else if (e.key === 'ArrowDown' || (mod && e.key.toLowerCase() === 'n')) {
      e.preventDefault(); e.stopPropagation(); moveSel(1);
    } else if (e.key === 'ArrowUp' || (mod && e.key.toLowerCase() === 'p')) {
      e.preventDefault(); e.stopPropagation(); moveSel(-1);
    } else if (mod) {
      // Keep global shortcuts (Ctrl+S, Ctrl+K, …) from firing under the palette.
      e.stopPropagation();
    }
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
      if (newOpen || deleteOpen || settingsOpen || helpOpen || cmdOpen) {
        newOpen = deleteOpen = settingsOpen = helpOpen = cmdOpen = false;
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
    const ctx = l && !slashOpen ? metaContext(l.text, l.col) : null;
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

  function fmtDate(s: string): string {
    const d = new Date(s);
    return isNaN(d.getTime()) ? '' : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }

  onMount(() => {
    loadInfo().catch((e) => flash(errMsg(e), true));
    refreshList().then(openStartPage);
    loadUserSlash();
    loadMetaStats();
    window.addEventListener('focus', onFocus);
    // Ctrl+Shift+Z is bound in main.go (Wails swallows it on Linux).
    window.addEventListener('katnotes:toggle-zoom', toggleZoom);
    window.addEventListener('beforeunload', () => { if (dirty) save(); });
    return () => {
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

    <input
      class="search"
      type="search"
      placeholder="Search…  (Ctrl+K)"
      bind:value={query}
      bind:this={searchEl}
      oninput={onSearchInput}
    />

    <nav class="tabs">
      <button class:active={tab === 'page'} onclick={() => switchTab('page')}>Pages</button>
      <button class:active={tab === 'journal'} onclick={() => switchTab('journal')}>Journal</button>
      <button class:active={tab === 'tags'} onclick={() => switchTab('tags')}>Tags</button>
      <button class:active={tab === 'tasks'} onclick={() => switchTab('tasks')}>Tasks</button>
    </nav>

    {#if tagFilter || query}
      <div class="filters">
        {#if tagFilter}<span class="chip">#{tagFilter}</span>{/if}
        {#if query}<span class="chip">“{query}”</span>{/if}
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
        {#each items as n (keyOf(n))}
          {@const m = matches[keyOf(n)]}
          <li>
            <button
              class="item"
              class:active={current && keyOf(current) === keyOf(n)}
              onclick={() => open(n.kind, n.id)}
            >
              <span class="title">{n.title}</span>
              <span class="meta">
                {n.kind === 'journal' ? n.id : fmtDate(n.updated)}
                {#each n.tags ?? [] as t}<span class="tag">#{t}</span>{/each}
              </span>
              {#if m && m.lines && m.lines.length}
                <span class="snippet">…{m.lines[0].text}</span>
              {:else if n.snippet}
                <span class="snippet">{n.snippet}</span>
              {/if}
            </button>
          </li>
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
            oninput={() => { onEdit(); updateSlash(); updateMeta(); }}
            onkeydown={onBodyKey}
            onkeyup={(e) => { if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) { updateSlash(); updateMeta(); } }}
            onclick={() => { updateSlash(); updateMeta(); }}
            onblur={() => { slashOpen = false; metaOpen = false; }}
            onscroll={() => { if (slashOpen) updateSlash(); if (metaOpen) updateMeta(); }}
            spellcheck="true"
          ></textarea>
          {#if slashOpen && slashItems.length}
            <ul class="slash-menu" class:above={slashPos.above} style:left="{slashPos.left}px" style:top="{slashPos.top}px" role="listbox">
              {#each slashItems as c, i (c.name)}
                <li role="option" aria-selected={i === slashSel}>
                  <button class="slash-item" class:active={i === slashSel} tabindex="-1"
                    onmousedown={(e) => { e.preventDefault(); slashSel = i; acceptSlash(c); }}
                    onmouseenter={() => (slashSel = i)}>
                    <span class="cmd-name">/{c.name}</span>
                    <span class="cmd-desc">{c.desc()}</span>
                    {#if c.user}<span class="lua-badge">lua</span>{/if}
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
          {#if metaOpen && metaItems.length}
            <ul class="slash-menu meta-menu" class:above={metaPos.above} style:left="{metaPos.left}px" style:top="{metaPos.top}px" role="listbox">
              {#each metaItems as sug, i (sug.label + sug.insert)}
                <li role="option" aria-selected={i === metaSel}>
                  <button class="slash-item" class:active={i === metaSel} tabindex="-1"
                    onmousedown={(e) => { e.preventDefault(); metaSel = i; acceptMeta(sug); }}
                    onmouseenter={() => (metaSel = i)}>
                    <span class="cmd-name">{sug.label}{sug.then ? ':' : ''}</span>
                    <span class="cmd-desc">{sug.detail}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
          </div>
        {/if}
        {#if viewMode !== 'edit'}
          <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
          <article class="preview" onclick={onPreviewClick}>{@html preview}</article>
        {/if}
      </section>
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
      <div class="cmd-line">
        <span class="cmd-prompt">:</span>
        <div class="cmd-field">
          <span class="cmd-ghost" aria-hidden="true"><span class="cmd-typed">{cmdInput}</span>{cmdGhost}</span>
          <input
            bind:this={cmdEl}
            bind:value={cmdInput}
            oninput={() => (cmdSel = 0)}
            onkeydown={onCmdKey}
            spellcheck="false"
            autocomplete="off"
            aria-label="Command"
          />
        </div>
      </div>
      <ul class="cmd-list">
        {#each cmdMatches as c, i (c.name)}
          <li>
            <button class="cmd-item" class:active={i === cmdSel} onmouseenter={() => (cmdSel = i)} onclick={() => { cmdSel = i; cmdInput = c.name; runCommand(); }}>
              <span class="cmd-name">{c.name}</span>
              <span class="cmd-desc">{c.desc}</span>
              {#if c.keys}<kbd>{c.keys}</kbd>{/if}
            </button>
          </li>
        {:else}
          <li class="empty">No matching command.</li>
        {/each}
      </ul>
      <div class="cmd-foot hint">Tab complete · ↑↓ select · Enter run · Esc close</div>
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
          {#each commands as c}
            <dt><kbd>:{c.name}</kbd></dt>
            <dd>{c.desc}</dd>
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
