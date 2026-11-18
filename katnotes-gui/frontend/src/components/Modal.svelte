<script lang="ts">
  import type { Snippet } from 'svelte';
  // A dialog over a backdrop; clicking the backdrop closes it.
  let {
    onclose,
    cls = '',
    label = '',
    onkeydown,
    children,
  }: {
    onclose: () => void;
    cls?: string;
    label?: string;
    onkeydown?: (e: KeyboardEvent) => void;
    children: Snippet;
  } = $props();
</script>

<div class="backdrop" role="presentation" onclick={onclose}>
  <div class="modal {cls}" role="dialog" aria-label={label || undefined} tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={onkeydown ?? (() => {})}>
    {@render children()}
  </div>
</div>
