<script>
  // Context help for the current page (F1). Non-modal side panel: the page
  // stays usable while the help is open.
  import { onMount } from 'svelte';
  import { route, href } from '../lib/router.svelte.js';
  import { ui } from '../lib/nav.svelte.js';
  import { helpPages, helpKey } from '../lib/help.js';

  const page = $derived(helpPages[helpKey(route)]);
  let heading = $state();
  let opener;
  onMount(() => {
    opener = document.activeElement;
    heading?.focus();
    return () => { if (opener instanceof HTMLElement && document.contains(opener)) opener.focus(); };
  });
  function close() { ui.helpOpen = false; }
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && !ui.paletteOpen && close()} />

<aside class="help no-print" aria-labelledby="help-title">
  <div class="head">
    <h2 id="help-title" tabindex="-1" bind:this={heading}>Hilfe: {page.title}</h2>
    <button class="ghost small" onclick={close} aria-label="Hilfe schließen"><span aria-hidden="true">✕</span></button>
  </div>
  <div class="body">
    <p>{page.purpose}</p>
    <h3>So gehen Sie vor</h3>
    <ul>{#each page.steps as s}<li>{s}</li>{/each}</ul>
    <p class="more">Alle Seiten, Tastenkürzel und Begriffe: <a href={href('/hilfe')} onclick={close}>Handbuch öffnen</a></p>
  </div>
</aside>

<style>
  .help { position: fixed; top: var(--topbar-h); right: 0; bottom: 0; width: min(380px, 100%); z-index: 40;
    background: var(--surface); border-left: 1px solid var(--line); box-shadow: var(--e-2); display: flex; flex-direction: column; }
  .head { display: flex; align-items: center; gap: var(--s-2); padding: var(--s-3) var(--s-4); border-bottom: 1px solid var(--line); }
  .head h2 { flex: 1; font-size: var(--t-lg); }
  .head h2:focus { outline: none; }
  .body { padding: var(--s-4); overflow: auto; display: flex; flex-direction: column; gap: var(--s-3); }
  h3 { font-size: var(--t-md); }
  ul { margin: 0; padding-left: var(--s-5); display: flex; flex-direction: column; gap: var(--s-2); }
  .more { font-size: var(--t-sm); color: var(--muted); border-top: 1px solid var(--line); padding-top: var(--s-3); }
</style>
