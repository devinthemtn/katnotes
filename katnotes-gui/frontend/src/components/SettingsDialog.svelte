<script lang="ts">
  import Modal from './Modal.svelte';
  import { keyOf } from '../links';
  import type { Config } from '../../bindings/github.com/devinthemtn/katnotes/internal/config';
  import type { Note } from '../../bindings/github.com/devinthemtn/katnotes/internal/notes';
  import type { ThemeInfo } from '../../bindings/github.com/devinthemtn/katnotes/katnotes-gui/models';

  let {
    cfg,
    themeInfo,
    current,
    startPages,
    configPath,
    version,
    onsave,
    onchoosedir,
    onerror,
    onclose,
  }: {
    cfg: Config;
    themeInfo: ThemeInfo | null;
    current: Note | null;
    startPages: Note[];
    configPath: string;
    version: string;
    onsave: (key: string, value: string) => Promise<void>;
    onchoosedir: () => void;
    onerror: (msg: string) => void;
    onclose: () => void;
  } = $props();

  let themeNames = $derived(themeInfo?.names ?? ['dark', 'light']);
  let startMode = $derived(['none', 'last', 'today'].includes(cfg.start_page || 'none') ? cfg.start_page || 'none' : 'note');

  async function setStartMode(mode: string) {
    if (mode !== 'note') return onsave('start_page', mode);
    const pick = current ? keyOf(current) : startPages[0] ? keyOf(startPages[0]) : '';
    if (!pick) { onerror('no pages to choose from yet'); return; }
    await onsave('start_page', pick);
  }

  const val = (e: Event) => (e.target as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement).value;
  const checked = (e: Event) => String((e.target as HTMLInputElement).checked);
</script>

{#snippet themeSelect(key: string, value: string, sameLabel: string)}
  <select {value} onchange={(e) => onsave(key, val(e))}>
    {#if sameLabel}<option value="">{sameLabel}</option>{/if}
    <option value="auto">Auto (light or dark, following the system)</option>
    {#each themeNames as name (name)}
      <option value={name}>{name}</option>
    {/each}
  </select>
{/snippet}

<Modal {onclose} cls="wide settings" label="Settings">
  <header class="settings-head"><h2>Settings</h2></header>
  <div class="settings-body">
  <label>Notes folder
    <div class="row">
      <input value={cfg.notes_dir} readonly />
      <button onclick={onchoosedir}>Change…</button>
    </div>
  </label>
  <div class="row font-row">
    <label>External editor (CLI/TUI)
      <input value={cfg.editor} placeholder="$VISUAL / $EDITOR" onchange={(e) => onsave('editor', val(e))} />
    </label>
    <label>New notes from the CLI
      <select value={cfg.default_kind || 'page'} onchange={(e) => onsave('default_kind', val(e))}>
        <option value="page">Page</option>
        <option value="journal">Journal entry</option>
      </select>
    </label>
  </div>
  <label>Open on launch
    <div class="row">
      <select value={startMode} onchange={(e) => setStartMode(val(e))}>
        <option value="none">Welcome screen</option>
        <option value="last">Last opened note</option>
        <option value="today">Today's journal entry</option>
        <option value="note">A specific note…</option>
      </select>
      {#if startMode === 'note'}
        <select value={cfg.start_page} onchange={(e) => onsave('start_page', val(e))}>
          {#if current && current.kind === 'journal'}
            <option value={keyOf(current)}>{current.title} (journal)</option>
          {/if}
          {#if !startPages.some((p) => keyOf(p) === cfg.start_page) && !(current && keyOf(current) === cfg.start_page)}
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
    <label>Shared theme
      {@render themeSelect('theme', cfg.theme || 'auto', '')}
    </label>
    <label>GUI theme
      {@render themeSelect('gui_theme', cfg.gui_theme ?? '', `Same as shared (${cfg.theme || 'auto'})`)}
    </label>
    <label>TUI theme
      {@render themeSelect('tui_theme', cfg.tui_theme ?? '', `Same as shared (${cfg.theme || 'auto'})`)}
    </label>
  </div>
  {#if themeInfo?.dir}<p class="hint">The shared theme is used by the TUI and the GUI unless one sets its own. Themes are .toml files in {themeInfo.dir}; copy light.toml or dark.toml to make your own.</p>{/if}
  <div class="row font-row">
    <label>Body font
      <input value={cfg.font?.body ?? ''} placeholder="system default" onchange={(e) => onsave('font.body', val(e))} />
    </label>
    <label>Mono font
      <input value={cfg.font?.mono ?? ''} placeholder="system monospace" onchange={(e) => onsave('font.mono', val(e))} />
    </label>
    <label class="size">Size (px)
      <input type="number" min="8" max="40" value={cfg.font?.size ?? 14} onchange={(e) => onsave('font.size', val(e))} />
    </label>
  </div>
  <label class="check">
    <input type="checkbox" checked={cfg.frontmatter_collapsed} onchange={(e) => onsave('frontmatter_collapsed', checked(e))} />
    Start with the frontmatter table collapsed
  </label>
  <label class="check">
    <input type="checkbox" checked={cfg.task_done_date !== false} onchange={(e) => onsave('task_done_date', checked(e))} />
    Add {'{'}done:date{'}'} when a task is checked
  </label>
  <div class="row font-row">
    <label>Open notes with (:external)
      <input value={cfg.open_with} placeholder="system default (xdg-open)" onchange={(e) => onsave('open_with', val(e))} />
    </label>
    <label>…in the TUI
      <input value={cfg.tui_open_with ?? ''} placeholder="same as open_with" onchange={(e) => onsave('tui_open_with', val(e))} />
    </label>
  </div>
  <div class="row font-row">
    <label>TUI previewer (o key)
      <input value={cfg.previewer ?? ''} placeholder="katdown, glow, …" onchange={(e) => onsave('previewer', val(e))} />
    </label>
    <label>…runs in
      <select value={cfg.previewer_mode || 'auto'} onchange={(e) => onsave('previewer_mode', val(e))}>
        <option value="auto">Auto</option>
        <option value="terminal">The terminal</option>
        <option value="window">Its own window</option>
      </select>
    </label>
  </div>
  <label>Journal date format
    <input value={cfg.date_format} placeholder="yyyy-mm-dd" onchange={(e) => onsave('date_format', val(e))} />
    <span class="hint" style="word-break: normal">A pattern like dd/mm/yyyy, mm/dd/yyyy or dddd d mmmm yyyy (or a Go layout). Journal titles use it, and new journal entries accept dates typed that way.</span>
  </label>
  <label>Journal template
    <textarea rows="4" value={cfg.journal_template} onchange={(e) => onsave('journal_template', val(e))}></textarea>
  </label>
  <p class="hint">Config: {configPath} · katnotes-gui {version}</p>
  </div>
  <footer class="settings-foot">
    <button class="primary" onclick={onclose}>Done</button>
  </footer>
</Modal>
