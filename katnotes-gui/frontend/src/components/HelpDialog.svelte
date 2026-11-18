<script lang="ts">
  import Modal from './Modal.svelte';
  import appIcon from '../../../build/appicon.svg';
  import type { Spec as CmdSpec } from '../../bindings/github.com/devinthemtn/katnotes/internal/commands';
  let {
    sections,
    catalog,
    cmdKeys,
    version,
    onclose,
  }: {
    sections: { title: string; keys: [string, string][] }[];
    catalog: CmdSpec[];
    cmdKeys: Record<string, string>;
    version: string;
    onclose: () => void;
  } = $props();

  function usage(c: CmdSpec): string {
    return [c.name, ...(c.args ?? []).map((a) => {
      const n = a.name + (a.rest || a.many ? '…' : '');
      return a.optional ? `[${n}]` : `<${n}>`;
    })].join(' ');
  }
</script>

<Modal {onclose} cls="help" label="Keyboard help">
  <header class="help-head">
    <div class="help-brand">
      <img class="help-icon" src={appIcon} alt="" width="44" height="44" />
      <div>
        <div class="help-title">Keyboard help</div>
        <div class="help-sub">KatNotes desktop</div>
      </div>
    </div>
    <span class="version-pill">katnotes-gui {version || 'dev'}</span>
  </header>
  <div class="help-body">
    {#each sections as sec (sec.title)}
      <h3>{sec.title}</h3>
      <dl>
        {#each sec.keys as [k, d], i (i)}
          <dt><kbd>{k}</kbd></dt>
          <dd>{d}</dd>
        {/each}
      </dl>
    {/each}
    <h3>Command palette</h3>
    <dl>
      {#each catalog as c (c.name)}
        <dt><kbd>:{usage(c)}</kbd></dt>
        <dd>{c.desc}{#if cmdKeys[c.name]} <span class="hint">({cmdKeys[c.name]})</span>{/if}</dd>
      {/each}
    </dl>
  </div>
  <footer class="help-foot">
    <span class="hint">Esc or ? to close</span>
    <button class="primary" onclick={onclose}>Close</button>
  </footer>
</Modal>
