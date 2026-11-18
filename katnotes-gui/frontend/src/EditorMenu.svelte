<script module lang="ts">
  // One row of an editor dropdown (slash commands, {…} metadata, [[ links).
  export type MenuItem = { key: string; label: string; detail?: string; badge?: string; muted?: boolean };
  export type MenuPos = { left: number; top: number; above: boolean };
</script>

<script lang="ts">
  let {
    items,
    sel,
    pos,
    wide = false,
    onpick,
    onhover,
  }: {
    items: MenuItem[];
    sel: number;
    pos: MenuPos;
    wide?: boolean;
    onpick: (i: number) => void;
    onhover: (i: number) => void;
  } = $props();
</script>

<ul class="slash-menu" class:wide class:above={pos.above} style:left="{pos.left}px" style:top="{pos.top}px" role="listbox">
  {#each items as it, i (it.key)}
    <li role="option" aria-selected={i === sel}>
      <button
        class="slash-item"
        class:active={i === sel}
        class:muted={it.muted}
        tabindex="-1"
        onmousedown={(e) => { e.preventDefault(); onpick(i); }}
        onmouseenter={() => onhover(i)}
      >
        <span class="cmd-name">{it.label}</span>
        {#if it.detail}<span class="cmd-desc">{it.detail}</span>{/if}
        {#if it.badge}<span class="lua-badge">{it.badge}</span>{/if}
      </button>
    </li>
  {/each}
</ul>
