<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from './Modal.svelte';
  let {
    journal,
    datePattern = 'YYYY-MM-DD',
    templates,
    tagFilter,
    title = $bindable(''),
    template = $bindable(''),
    oncreate,
    onclose,
  }: {
    journal: boolean;
    datePattern?: string;
    templates: string[];
    tagFilter: string;
    title: string;
    template: string;
    oncreate: () => void;
    onclose: () => void;
  } = $props();
  let inputEl: HTMLInputElement | undefined = $state();
  onMount(() => inputEl?.focus());
</script>

<Modal {onclose} label={journal ? 'New journal entry' : 'New page'}>
  <form onsubmit={(e) => { e.preventDefault(); oncreate(); }}>
    <h2>{journal ? 'New journal entry' : 'New page'}</h2>
    <input bind:this={inputEl} bind:value={title} placeholder={journal ? `${datePattern} (empty for today)` : 'Title'} />
    {#if !journal && templates.length}
      <label class="tmpl-pick">Template
        <select bind:value={template}>
          <option value="">Blank page</option>
          {#each templates as t (t)}<option value={t}>{t}</option>{/each}
        </select>
      </label>
    {/if}
    {#if tagFilter}<p class="hint">Will be tagged #{tagFilter}</p>{/if}
    <div class="modal-actions">
      <button type="button" onclick={onclose}>Cancel</button>
      <button type="submit" class="primary">Create</button>
    </div>
  </form>
</Modal>
