<script lang="ts">
  import Modal from './Modal.svelte';
  let { title, path, ondelete, onclose }: { title: string; path: string; ondelete: () => void; onclose: () => void } = $props();
</script>

<Modal {onclose} label="Delete note">
  <h2>Delete “{title}”?</h2>
  <p class="hint">{path}</p>
  <div class="modal-actions">
    <button onclick={onclose}>Cancel</button>
    <button class="danger" onclick={ondelete}>Delete</button>
  </div>
</Modal>
