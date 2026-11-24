<script lang="ts">
  import Modal from './Modal.svelte';
  import { NotesService as api } from '../../bindings/github.com/devinthemtn/katnotes/katnotes-gui';

  // Shown when something needs todo.txt sync ([todo_txt] in config.toml)
  // while it is off: explains it and turns it on.
  let {
    file: initialFile,
    onenable,
    onclose,
  }: { file: string; onenable: (file: string) => void; onclose: () => void } = $props();

  // svelte-ignore state_referenced_locally
  let file = $state(initialFile);
  let resolved = $state<{ todo: string; done: string; todoExists: boolean } | null>(null);

  $effect(() => {
    const f = file.trim();
    const t = setTimeout(async () => {
      try {
        resolved = await api.TodoFilesFor(f);
      } catch {
        resolved = null;
      }
    }, 150);
    return () => clearTimeout(t);
  });
</script>

<Modal {onclose} label="Link tasks to katTasks" cls="todo-setup">
  <h2>Link tasks to katTasks (todo.txt)</h2>
  <p>
    Linking to todo.txt is turned off. When it's on, a task with <code>{'{'}kt:new{'}'}</code> (or ⇄ in the
    all-tasks view) gets an ID and a line in the todo.txt file katTasks uses. Saving the note updates that line,
    and tasks checked in katTasks come back to the note.
  </p>
  <label class="field">todo.txt file
    <input bind:value={file} placeholder="empty: katTasks' files.todo, else ~/todo.txt" />
  </label>
  {#if resolved}
    <p class="hint">
      Will use <code>{resolved.todo}</code>
      {#if !resolved.todoExists}(doesn't exist yet; it will be created){/if}
      <br />Done tasks: <code>{resolved.done}</code>
    </p>
  {/if}
  <p class="hint">
    This sets <code>enabled = true</code> under <code>[todo_txt]</code> in config.toml. You can change it later
    in Settings.
  </p>
  <div class="modal-actions">
    <button onclick={onclose}>Not now</button>
    <button class="primary" onclick={() => onenable(file.trim())}>Turn on linking</button>
  </div>
</Modal>

<style>
  .field { display: flex; flex-direction: column; gap: 4px; margin: 8px 0; }
  .field input { width: 100%; }
  code { word-break: break-all; }
</style>
