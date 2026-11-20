<script>
  import Modal from './Modal.svelte';
  import { shortcuts } from '../lib/help.js';
  import { prefs } from '../lib/prefs.svelte.js';
  import { href } from '../lib/router.svelte.js';
  import { ui } from '../lib/nav.svelte.js';
</script>

<Modal title="Tastenkürzel" onclose={() => (ui.shortcutsOpen = false)}>
  <div class="stack">
    {#if !prefs.shortcuts}
      <p class="info-box">Einzeltasten-Kürzel sind ausgeschaltet. Strg+K, F1 und Esc funktionieren immer. Einschalten unter <a href={href('/einstellungen')} onclick={() => (ui.shortcutsOpen = false)}>Einstellungen</a>.</p>
    {/if}
    <table>
      <thead><tr><th>Tasten</th><th>Wirkung</th></tr></thead>
      <tbody>
        {#each shortcuts as s}
          <tr class:off={s.single && !prefs.shortcuts}>
            <td class="nowrap">{#each s.keys as k, i}{#if i}<span class="then"> {s.keys[0] === 'Strg' ? '+' : 'dann'} </span>{/if}<kbd>{k}</kbd>{/each}</td>
            <td>{s.text}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</Modal>

<style>
  .then { color: var(--muted); font-size: var(--t-sm); }
  tr.off td { color: var(--muted); }
</style>
