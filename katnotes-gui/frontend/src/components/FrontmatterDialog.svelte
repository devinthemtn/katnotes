<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from './Modal.svelte';
  let {
    noteTitle,
    inferred,
    error,
    modLabel,
    text = $bindable(''),
    onsave,
    onclose,
  }: {
    noteTitle: string;
    inferred: boolean;
    error: string;
    modLabel: string;
    text: string;
    onsave: () => void;
    onclose: () => void;
  } = $props();
  let textEl: HTMLTextAreaElement | undefined = $state();
  onMount(() => textEl?.focus());

  function onkey(e: KeyboardEvent) {
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); onclose(); }
    else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); e.stopPropagation(); onsave(); }
    else if (e.ctrlKey || e.metaKey) e.stopPropagation();
  }
</script>

<Modal {onclose} cls="fm-edit" label="Edit frontmatter" onkeydown={onkey}>
  <h2>Frontmatter · {noteTitle}</h2>
  {#if inferred}
    <p class="hint">This note has no frontmatter yet. These values were worked out from the file (first heading, file dates); saving adds them to it.</p>
  {/if}
  <textarea bind:this={textEl} bind:value={text} spellcheck="false" rows="12"></textarea>
  <p class="hint">YAML. <code>tags: [a, b]</code> · <code>created: 2026-01-31</code> · <code>updated</code> is set when you save · a new title renames the note.</p>
  {#if error}<div class="cmd-err">{error}</div>{/if}
  <div class="modal-actions">
    <button onclick={onclose}>Cancel</button>
    <button class="primary" onclick={onsave} title="{modLabel}+Enter">Save</button>
  </div>
</Modal>
