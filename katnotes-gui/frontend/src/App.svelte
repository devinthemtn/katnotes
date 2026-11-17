<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import { NotesService as api } from '../bindings/github.com/devinthemtn/katnotes/katnotes-gui';
  import type { Note, Match, TagCount } from '../bindings/github.com/devinthemtn/katnotes/internal/notes';
  import type { Config } from '../bindings/github.com/devinthemtn/katnotes/internal/config';

  type Tab = 'page' | 'journal' | 'tags';
  type ViewMode = 'edit' | 'split' | 'preview';

  let tab: Tab = $state('page');
  let items: Note[] = $state([]);
  let tags: TagCount[] = $state([]);
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

  const isMac = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform);
  const modLabel = isMac ? '⌘' : 'Ctrl';
  const helpSections: { title: string; keys: [string, string][] }[] = [
    { title: 'Notes', keys: [
      [`${modLabel}+N`, 'New page (or journal entry on the Journal tab)'],
      [`${modLabel}+J`, "Open today's journal entry"],
      [`${modLabel}+S`, 'Save now (notes also autosave as you type)'],
      ['Enter', 'In the title field: rename the note'],
      ['Tab', 'In the editor: indent two spaces'],
    ]},
    { title: 'Finding', keys: [
      [`${modLabel}+K / ${modLabel}+F`, 'Focus search'],
      ['Tags tab', 'Click a tag to filter Pages and Journal'],
      ['clear', 'Remove the search and tag filter'],
    ]},
    { title: 'View', keys: [
      [`${modLabel}+E`, 'Cycle Edit → Split → Preview'],
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
    DOMPurify.sanitize(marked.parse(draftBody, { async: false, gfm: true, breaks: false }) as string)
  );

  function parseTags(s: string): string[] {
    return s.split(/[,\s]+/).map((t) => t.replace(/^#/, '').trim().toLowerCase()).filter(Boolean);
  }

  async function refreshList() {
    try {
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
    current = n;
    conflict = false;
    dirty = false;
    saveState = 'saved';
    draftBody = n?.body ?? '';
    draftTags = (n?.tags ?? []).join(', ');
    draftTitle = n?.title ?? '';
  }

  async function open(kind: string, id: string) {
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
      if (tab === 'tags') tab = 'page';
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
    if (tab === 'tags') tab = 'page';
    await refreshList();
  }

  async function clearFilters() {
    tagFilter = '';
    query = '';
    await refreshList();
  }

  function setView(m: ViewMode) {
    viewMode = m;
    savePref('viewMode', m);
  }

  async function openSettings() {
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

  function isTyping(t: EventTarget | null): boolean {
    const el = t as HTMLElement | null;
    if (!el) return false;
    return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName);
  }

  function onKey(e: KeyboardEvent) {
    const mod = e.ctrlKey || e.metaKey;
    if (e.key === 'Escape') {
      if (newOpen || deleteOpen || settingsOpen || helpOpen) {
        newOpen = deleteOpen = settingsOpen = helpOpen = false;
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
    switch (e.key.toLowerCase()) {
      case 's': e.preventDefault(); save(true); break;
      case 'n': e.preventDefault(); openNew(); break;
      case 'k':
      case 'f': e.preventDefault(); searchEl?.focus(); searchEl?.select(); break;
      case 'j': e.preventDefault(); openToday(); break;
      case 'e': e.preventDefault(); setView(viewMode === 'edit' ? 'split' : viewMode === 'split' ? 'preview' : 'edit'); break;
    }
  }

  function onBodyKey(e: KeyboardEvent) {
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
    refreshList();
    window.addEventListener('focus', onFocus);
    window.addEventListener('beforeunload', () => { if (dirty) save(); });
    return () => window.removeEventListener('focus', onFocus);
  });
</script>

<svelte:window onkeydown={onKey} />

<div class="app">
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
    </nav>

    {#if tagFilter || query}
      <div class="filters">
        {#if tagFilter}<span class="chip">#{tagFilter}</span>{/if}
        {#if query}<span class="chip">“{query}”</span>{/if}
        <button class="link" onclick={clearFilters}>clear</button>
      </div>
    {/if}

    <ul class="list">
      {#if tab === 'tags'}
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

  <main class="main">
    {#if current}
      <header class="note-head">
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
          <textarea
            class="editor"
            bind:this={bodyEl}
            bind:value={draftBody}
            oninput={onEdit}
            onkeydown={onBodyKey}
            spellcheck="true"
          ></textarea>
        {/if}
        {#if viewMode !== 'edit'}
          <article class="preview">{@html preview}</article>
        {/if}
      </section>
    {:else}
      <div class="welcome">
        <h1>KatNotes</h1>
        <p>Select a note, or start something new.</p>
        <div class="actions">
          <button class="primary" onclick={openNew}>New page</button>
          <button onclick={openToday}>Today's journal</button>
        </div>
        <p class="keys">Ctrl+N new · Ctrl+J today · Ctrl+K search · Ctrl+S save · Ctrl+E toggle preview</p>
      </div>
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
