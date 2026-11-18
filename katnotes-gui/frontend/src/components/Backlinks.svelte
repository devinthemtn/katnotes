<script lang="ts">
  import type { Backlink } from '../../bindings/github.com/devinthemtn/katnotes/internal/notes';
  import { keyOf, plainLinks } from '../links';
  let {
    backlinks,
    onopen,
    onopenline,
  }: {
    backlinks: Backlink[];
    onopen: (kind: string, id: string) => void;
    onopenline: (kind: string, id: string, line: number) => void;
  } = $props();
</script>

<details class="backlinks" open>
  <summary>Linked from <span class="backlinks-count">{backlinks.length}</span></summary>
  <ul>
    {#each backlinks as b (keyOf(b.note))}
      <li>
        <button class="backlink-note" onclick={() => onopen(b.note.kind, b.note.id)}>
          {b.note.title}<span class="task-note-meta"> · {b.note.kind === 'journal' ? b.note.id : 'page'}</span>
        </button>
        {#each b.lines ?? [] as l (l.line)}
          <button class="backlink-line" onclick={() => onopenline(b.note.kind, b.note.id, l.line - 1)} title="Open at this line">{plainLinks(l.text)}</button>
        {/each}
      </li>
    {/each}
  </ul>
</details>
